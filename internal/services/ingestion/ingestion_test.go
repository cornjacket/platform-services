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

func startIngestion(t *testing.T, mock *channelSubmitter, errorCh chan<- error) *RunningService {
	t.Helper()
	ctx := context.Background()

	svc, err := Start(ctx, Config{
		Port:         testPort,
		WorkerCount:  1,
		BatchSize:    10,
		MaxRetries:   3,
		PollInterval: 100 * time.Millisecond,
		DatabaseURL:  testDBURL,
	}, testPool, mock, testLogger(), errorCh)
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
	startIngestion(t, mock, nil) // Pass nil for errorCh

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
	startIngestion(t, mock, nil) // Pass nil for errorCh

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
	startIngestion(t, mock, nil) // Pass nil for errorCh

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

func TestIngestion_PortCollisionShutdown(t *testing.T) {
	mock := &channelSubmitter{calls: make(chan *events.Envelope, 1)}
	errorCh := make(chan error, 1)

	// Start first instance (should succeed)
	svc1, err := Start(context.Background(), Config{
		Port:         testPort,
		WorkerCount:  1,
		BatchSize:    10,
		MaxRetries:   3,
		PollInterval: 100 * time.Millisecond,
		DatabaseURL:  testDBURL,
	}, testPool, mock, testLogger(), errorCh)
	require.NoError(t, err)
	defer svc1.Shutdown(context.Background())

	// Give server time to bind
	time.Sleep(50 * time.Millisecond)

	// Attempt to start second instance on the same port (should fail)
	svc2, err := Start(context.Background(), Config{
		Port:         testPort,
		WorkerCount:  1,
		BatchSize:    10,
		MaxRetries:   3,
		PollInterval: 100 * time.Millisecond,
		DatabaseURL:  testDBURL,
	}, testPool, mock, testLogger(), errorCh)
	require.NoError(t, err, "second service start should not return error directly")
	defer svc2.Shutdown(context.Background()) // No-op if not started properly

	// Verify an error is reported on the errorCh
	select {
	case reportedErr := <-errorCh:
		assert.Error(t, reportedErr)
		assert.Contains(t, reportedErr.Error(), fmt.Sprintf("ingestion server failed: listen tcp :%d: bind: address already in use", testPort))
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for port collision error")
	}
}