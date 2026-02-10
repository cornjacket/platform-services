//go:build integration

package projections

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
	testutil.MustDropAllTables(pool)
	testutil.MustRunMigrations(pool, "../../services/eventhandler/migrations")
	testPool = pool
	defer pool.Close()
	os.Exit(m.Run())
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func testEnvelope(t *testing.T, eventTime time.Time) *events.Envelope {
	t.Helper()
	return &events.Envelope{
		EventID:     uuid.Must(uuid.NewV7()),
		EventType:   "sensor.reading",
		AggregateID: "device-001",
		EventTime:   eventTime,
		IngestedAt:  time.Now().UTC().Truncate(time.Microsecond),
		Payload:     json.RawMessage(`{"temperature": 22.5}`),
		Metadata:    events.Metadata{Source: "test", SchemaVersion: 1},
	}
}

func TestWriteProjection_Insert(t *testing.T) {
	testutil.TruncateTables(t, testPool, "projections")
	store := NewPostgresStore(testPool, testLogger())

	env := testEnvelope(t, time.Now().UTC().Truncate(time.Microsecond))
	state := json.RawMessage(`{"status": "active"}`)

	err := store.WriteProjection(context.Background(), "sensor_state", "device-001", state, env)
	require.NoError(t, err)

	// Verify row was created
	p, err := store.GetProjection(context.Background(), "sensor_state", "device-001")
	require.NoError(t, err)
	assert.Equal(t, "sensor_state", p.ProjectionType)
	assert.Equal(t, "device-001", p.AggregateID)
	assert.JSONEq(t, `{"status": "active"}`, string(p.State))
	assert.Equal(t, env.EventID, p.LastEventID)
}

func TestWriteProjection_UpdateNewer(t *testing.T) {
	testutil.TruncateTables(t, testPool, "projections")
	store := NewPostgresStore(testPool, testLogger())

	oldTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	envOld := testEnvelope(t, oldTime)
	envNew := testEnvelope(t, newTime)

	// Write old event first
	require.NoError(t, store.WriteProjection(context.Background(),
		"sensor_state", "device-001", json.RawMessage(`{"v": 1}`), envOld))

	// Write newer event — should update
	require.NoError(t, store.WriteProjection(context.Background(),
		"sensor_state", "device-001", json.RawMessage(`{"v": 2}`), envNew))

	p, err := store.GetProjection(context.Background(), "sensor_state", "device-001")
	require.NoError(t, err)
	assert.JSONEq(t, `{"v": 2}`, string(p.State))
	assert.Equal(t, envNew.EventID, p.LastEventID)
}

func TestWriteProjection_SkipOlder(t *testing.T) {
	testutil.TruncateTables(t, testPool, "projections")
	store := NewPostgresStore(testPool, testLogger())

	oldTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	envNew := testEnvelope(t, newTime)
	envOld := testEnvelope(t, oldTime)

	// Write newer event first
	require.NoError(t, store.WriteProjection(context.Background(),
		"sensor_state", "device-001", json.RawMessage(`{"v": "new"}`), envNew))

	// Write older event — should be skipped by WHERE clause
	require.NoError(t, store.WriteProjection(context.Background(),
		"sensor_state", "device-001", json.RawMessage(`{"v": "old"}`), envOld))

	p, err := store.GetProjection(context.Background(), "sensor_state", "device-001")
	require.NoError(t, err)
	assert.JSONEq(t, `{"v": "new"}`, string(p.State), "older event should not overwrite newer projection")
	assert.Equal(t, envNew.EventID, p.LastEventID, "last_event_id should still be the newer event")
}

func TestWriteProjection_SameTimestamp_UUIDTiebreaker(t *testing.T) {
	testutil.TruncateTables(t, testPool, "projections")
	store := NewPostgresStore(testPool, testLogger())

	sameTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	env1 := testEnvelope(t, sameTime)
	env2 := testEnvelope(t, sameTime)

	// Determine which UUID is larger (for expected winner)
	var first, second *events.Envelope
	var expectedState string
	if env1.EventID.String() < env2.EventID.String() {
		first, second = env1, env2
		expectedState = `{"v": "second"}`
	} else {
		first, second = env2, env1
		expectedState = `{"v": "first"}`
	}

	// Write smaller UUID first
	require.NoError(t, store.WriteProjection(context.Background(),
		"sensor_state", "device-001", json.RawMessage(`{"v": "first"}`), first))

	// Write larger UUID — should win the tiebreaker
	require.NoError(t, store.WriteProjection(context.Background(),
		"sensor_state", "device-001", json.RawMessage(`{"v": "second"}`), second))

	p, err := store.GetProjection(context.Background(), "sensor_state", "device-001")
	require.NoError(t, err)
	assert.JSONEq(t, expectedState, string(p.State))
}

func TestGetProjection(t *testing.T) {
	testutil.TruncateTables(t, testPool, "projections")
	store := NewPostgresStore(testPool, testLogger())

	env := testEnvelope(t, time.Now().UTC().Truncate(time.Microsecond))
	state := json.RawMessage(`{"temperature": 22.5}`)
	require.NoError(t, store.WriteProjection(context.Background(),
		"sensor_state", "device-042", state, env))

	p, err := store.GetProjection(context.Background(), "sensor_state", "device-042")
	require.NoError(t, err)
	assert.Equal(t, "sensor_state", p.ProjectionType)
	assert.Equal(t, "device-042", p.AggregateID)
	assert.JSONEq(t, `{"temperature": 22.5}`, string(p.State))
	assert.Equal(t, env.EventID, p.LastEventID)
	assert.True(t, env.EventTime.Equal(p.LastEventTimestamp),
		"timestamp mismatch: %v vs %v", env.EventTime, p.LastEventTimestamp)
	assert.False(t, p.UpdatedAt.IsZero())
	assert.False(t, p.ProjectionID.IsNil())
}

func TestGetProjection_NotFound(t *testing.T) {
	testutil.TruncateTables(t, testPool, "projections")
	store := NewPostgresStore(testPool, testLogger())

	_, err := store.GetProjection(context.Background(), "sensor_state", "nonexistent")
	require.Error(t, err)
}

func TestListProjections(t *testing.T) {
	testutil.TruncateTables(t, testPool, "projections")
	store := NewPostgresStore(testPool, testLogger())

	// Insert 3 projections
	for i := 0; i < 3; i++ {
		env := testEnvelope(t, time.Now().UTC().Truncate(time.Microsecond))
		env.AggregateID = "device-" + string(rune('A'+i))
		require.NoError(t, store.WriteProjection(context.Background(),
			"sensor_state", env.AggregateID, json.RawMessage(`{}`), env))
	}

	// List with pagination: limit 2, offset 0
	results, total, err := store.ListProjections(context.Background(), "sensor_state", 2, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, results, 2)

	// List with offset 2 — should get 1
	results, total, err = store.ListProjections(context.Background(), "sensor_state", 2, 2)
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, results, 1)
}

func TestListProjections_Empty(t *testing.T) {
	testutil.TruncateTables(t, testPool, "projections")
	store := NewPostgresStore(testPool, testLogger())

	results, total, err := store.ListProjections(context.Background(), "sensor_state", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.NotNil(t, results, "should return empty slice, not nil")
	assert.Empty(t, results)
}
