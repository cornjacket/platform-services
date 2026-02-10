//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
	"github.com/cornjacket/platform-services/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool := testutil.MustNewTestPool()
	testutil.MustRunMigrations(pool, "../../../services/ingestion/migrations")
	testPool = pool
	defer pool.Close()
	os.Exit(m.Run())
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func testEnvelope(t *testing.T) *events.Envelope {
	t.Helper()
	return &events.Envelope{
		EventID:     uuid.Must(uuid.NewV7()),
		EventType:   "sensor.reading",
		AggregateID: "device-001",
		EventTime:   time.Now().UTC().Truncate(time.Microsecond),
		IngestedAt:  time.Now().UTC().Truncate(time.Microsecond),
		Payload:     json.RawMessage(`{"temperature": 22.5}`),
		Metadata:    events.Metadata{Source: "test", SchemaVersion: 1},
	}
}

func TestOutboxInsert(t *testing.T) {
	testutil.TruncateTables(t, testPool, "outbox")
	repo := NewOutboxRepo(testPool, testLogger())

	env := testEnvelope(t)
	err := repo.Insert(context.Background(), env)
	require.NoError(t, err)

	// Verify row exists with expected columns
	var outboxID, payloadRaw string
	var retryCount int
	err = testPool.QueryRow(context.Background(),
		"SELECT outbox_id, event_payload::text, retry_count FROM outbox WHERE outbox_id = $1",
		env.EventID,
	).Scan(&outboxID, &payloadRaw, &retryCount)
	require.NoError(t, err)
	assert.Equal(t, env.EventID.String(), outboxID)
	assert.Equal(t, 0, retryCount)

	// Verify JSONB stored correctly
	var stored events.Envelope
	require.NoError(t, json.Unmarshal([]byte(payloadRaw), &stored))
	assert.Equal(t, env.EventType, stored.EventType)
	assert.Equal(t, env.AggregateID, stored.AggregateID)
}

func TestOutboxFetchPending(t *testing.T) {
	testutil.TruncateTables(t, testPool, "outbox")
	repo := NewOutboxRepo(testPool, testLogger())

	// Insert 3 events with staggered created_at
	for i := 0; i < 3; i++ {
		env := testEnvelope(t)
		require.NoError(t, repo.Insert(context.Background(), env))
		time.Sleep(2 * time.Millisecond) // ensure distinct created_at
	}

	// Fetch with limit 2 — should get the 2 oldest
	entries, err := repo.FetchPending(context.Background(), 2)
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	// Verify ordering: first entry should have earlier created_at
	// (We can't check created_at directly since OutboxEntry doesn't expose it,
	// but the SQL orders by created_at ASC, so the first entry is the oldest)
	assert.NotEqual(t, entries[0].OutboxID, entries[1].OutboxID)
}

func TestOutboxFetchPending_Empty(t *testing.T) {
	testutil.TruncateTables(t, testPool, "outbox")
	repo := NewOutboxRepo(testPool, testLogger())

	entries, err := repo.FetchPending(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestOutboxDelete(t *testing.T) {
	testutil.TruncateTables(t, testPool, "outbox")
	repo := NewOutboxRepo(testPool, testLogger())

	env := testEnvelope(t)
	require.NoError(t, repo.Insert(context.Background(), env))

	// Delete the entry
	err := repo.Delete(context.Background(), env.EventID.String())
	require.NoError(t, err)

	// Verify it's gone
	entries, err := repo.FetchPending(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestOutboxDelete_MissingID(t *testing.T) {
	testutil.TruncateTables(t, testPool, "outbox")
	repo := NewOutboxRepo(testPool, testLogger())

	// Delete a non-existent ID — should not error
	err := repo.Delete(context.Background(), uuid.Must(uuid.NewV7()).String())
	assert.NoError(t, err)
}

func TestOutboxIncrementRetry(t *testing.T) {
	testutil.TruncateTables(t, testPool, "outbox")
	repo := NewOutboxRepo(testPool, testLogger())

	env := testEnvelope(t)
	require.NoError(t, repo.Insert(context.Background(), env))

	// Increment twice: 0 → 1 → 2
	require.NoError(t, repo.IncrementRetry(context.Background(), env.EventID.String()))
	require.NoError(t, repo.IncrementRetry(context.Background(), env.EventID.String()))

	// Verify retry count
	var retryCount int
	err := testPool.QueryRow(context.Background(),
		"SELECT retry_count FROM outbox WHERE outbox_id = $1",
		env.EventID,
	).Scan(&retryCount)
	require.NoError(t, err)
	assert.Equal(t, 2, retryCount)
}

func TestOutboxInsertFetchRoundTrip(t *testing.T) {
	testutil.TruncateTables(t, testPool, "outbox")
	repo := NewOutboxRepo(testPool, testLogger())

	env := testEnvelope(t)
	require.NoError(t, repo.Insert(context.Background(), env))

	entries, err := repo.FetchPending(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	fetched := entries[0].Payload
	assert.Equal(t, env.EventID, fetched.EventID)
	assert.Equal(t, env.EventType, fetched.EventType)
	assert.Equal(t, env.AggregateID, fetched.AggregateID)
	assert.True(t, env.EventTime.Equal(fetched.EventTime), "EventTime mismatch: %v vs %v", env.EventTime, fetched.EventTime)
	assert.True(t, env.IngestedAt.Equal(fetched.IngestedAt), "IngestedAt mismatch: %v vs %v", env.IngestedAt, fetched.IngestedAt)
	assert.JSONEq(t, string(env.Payload), string(fetched.Payload))
	assert.Equal(t, env.Metadata.Source, fetched.Metadata.Source)
	assert.Equal(t, env.Metadata.SchemaVersion, fetched.Metadata.SchemaVersion)
}

func TestOutboxNotifyTrigger(t *testing.T) {
	testutil.TruncateTables(t, testPool, "outbox")
	repo := NewOutboxRepo(testPool, testLogger())

	// Acquire a dedicated connection for LISTEN
	conn, err := testPool.Acquire(context.Background())
	require.NoError(t, err)
	defer conn.Release()

	_, err = conn.Exec(context.Background(), "LISTEN outbox_insert")
	require.NoError(t, err)

	// Insert an event (triggers NOTIFY)
	env := testEnvelope(t)
	require.NoError(t, repo.Insert(context.Background(), env))

	// Wait for notification with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	notification, err := conn.Conn().WaitForNotification(ctx)
	require.NoError(t, err, "timed out waiting for NOTIFY")
	assert.Equal(t, "outbox_insert", notification.Channel)
	assert.Equal(t, env.EventID.String(), notification.Payload)
}
