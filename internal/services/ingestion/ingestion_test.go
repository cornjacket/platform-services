//go:build component

package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cornjacket/platform-services/internal/services/ingestion/worker"
	"github.com/cornjacket/platform-services/internal/shared/domain/events"
	"github.com/cornjacket/platform-services/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool := testutil.MustNewTestPool()
	testutil.MustDropAllTables(pool)
	testutil.MustRunMigrations(pool, "migrations")
	testPool = pool
	defer pool.Close()
	os.Exit(m.Run())
}

// channelSubmitter captures events submitted by the ingestion worker.
type channelSubmitter struct {
	calls chan *events.Envelope
}

func (m *channelSubmitter) SubmitEvent(_ context.Context, event *events.Envelope) error {
	m.calls <- event
	return nil
}

// Compile-time check: channelSubmitter implements worker.EventSubmitter.
var _ worker.EventSubmitter = (*channelSubmitter)(nil)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

const testDBURL = "postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable"
const testPort = 18080

func startIngestion(t *testing.T, mock *channelSubmitter) *RunningService {
	t.Helper()
	ctx := context.Background()

	svc, err := Start(ctx, Config{
		Port:         testPort,
		WorkerCount:  1,
		BatchSize:    10,
		MaxRetries:   3,
		PollInterval: 100 * time.Millisecond,
		DatabaseURL:  testDBURL,
	}, testPool, mock, testLogger())
	require.NoError(t, err)

	// Give server time to bind
	time.Sleep(50 * time.Millisecond)

	t.Cleanup(func() {
		svc.Shutdown(context.Background())
	})

	return svc
}

func postEvent(t *testing.T, body any) *http.Response {
	t.Helper()
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(
		fmt.Sprintf("http://localhost:%d/api/v1/events", testPort),
		"application/json",
		bytes.NewReader(jsonBody),
	)
	require.NoError(t, err)
	return resp
}

func TestIngestion_IngestToSubmit(t *testing.T) {
	testutil.TruncateTables(t, testPool, "outbox", "event_store")
	mock := &channelSubmitter{calls: make(chan *events.Envelope, 10)}
	startIngestion(t, mock)

	resp := postEvent(t, map[string]any{
		"event_type":   "sensor.reading",
		"aggregate_id": "device-001",
		"payload":      map[string]any{"temperature": 23.5},
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	select {
	case event := <-mock.calls:
		assert.Equal(t, "sensor.reading", event.EventType)
		assert.Equal(t, "device-001", event.AggregateID)
		assert.Contains(t, string(event.Payload), "temperature")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event submission")
	}
}

func TestIngestion_InvalidPayload(t *testing.T) {
	testutil.TruncateTables(t, testPool, "outbox", "event_store")
	mock := &channelSubmitter{calls: make(chan *events.Envelope, 10)}
	startIngestion(t, mock)

	// Post invalid JSON (missing required fields)
	resp := postEvent(t, map[string]any{
		"event_type": "sensor.reading",
		// missing aggregate_id and payload
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	// Confirm nothing was submitted
	select {
	case event := <-mock.calls:
		t.Fatalf("expected no submission, got event: %v", event.EventID)
	case <-time.After(500 * time.Millisecond):
		// Good — nothing submitted
	}
}

func TestIngestion_EventStoreWrite(t *testing.T) {
	testutil.TruncateTables(t, testPool, "outbox", "event_store")
	mock := &channelSubmitter{calls: make(chan *events.Envelope, 10)}
	startIngestion(t, mock)

	resp := postEvent(t, map[string]any{
		"event_type":   "sensor.reading",
		"aggregate_id": "device-persist",
		"payload":      map[string]any{"value": 42},
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	// Wait for worker to process (event reaches mock = full pipeline done)
	var submitted *events.Envelope
	select {
	case submitted = <-mock.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event submission")
	}

	// Verify event_store has the event
	var count int
	err := testPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM event_store WHERE event_id = $1", submitted.EventID,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
