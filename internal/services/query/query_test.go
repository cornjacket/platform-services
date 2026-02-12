//go:build component

package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
	"github.com/cornjacket/platform-services/internal/shared/projections"
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

const testPort = 18081

func startQuery(t *testing.T, errorCh chan<- error) *RunningService {
	t.Helper()
	ctx := context.Background()

	svc, err := Start(ctx, Config{Port: testPort}, testPool, testLogger(), errorCh)
	require.NoError(t, err)

	// Give server time to bind
	time.Sleep(50 * time.Millisecond)

	t.Cleanup(func() {
		// Ensure that the errorCh is not closed by the service before the test can read from it.
		// A common pattern is to just pass a buffered channel that the test then checks.
		// If the service sends an error, the test will catch it.
		svc.Shutdown(context.Background())
	})

	return svc
}

func seedProjection(t *testing.T, projType, aggregateID string, state map[string]any) {
	t.Helper()
	store := projections.NewPostgresStore(testPool, testLogger())

	stateJSON, err := json.Marshal(state)
	require.NoError(t, err)

	env := &events.Envelope{
		EventID:   uuid.Must(uuid.NewV7()),
		EventTime: time.Now().UTC().Truncate(time.Microsecond),
	}

	err = store.WriteProjection(context.Background(), projType, aggregateID, stateJSON, env)
	require.NoError(t, err)
}

func httpGet(t *testing.T, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d%s", testPort, path))
	require.NoError(t, err)
	return resp
}

func TestQuery_GetProjection(t *testing.T) {
	testutil.TruncateTables(t, testPool, "projections")
	startQuery(t, nil)

	seedProjection(t, "sensor_state", "device-100", map[string]any{"temperature": 22.0})

	resp := httpGet(t, "/api/v1/projections/sensor_state/device-100")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result Projection
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "sensor_state", result.ProjectionType)
	assert.Equal(t, "device-100", result.AggregateID)
	assert.JSONEq(t, `{"temperature":22}`, string(result.State))
}

func TestQuery_GetProjection_NotFound(t *testing.T) {
	testutil.TruncateTables(t, testPool, "projections")
	startQuery(t, nil)

	resp := httpGet(t, "/api/v1/projections/sensor_state/nonexistent")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestQuery_ListProjections(t *testing.T) {
	testutil.TruncateTables(t, testPool, "projections")
	startQuery(t, nil)

	// Seed 3 projections
	for i := 0; i < 3; i++ {
		seedProjection(t, "sensor_state", fmt.Sprintf("device-%d", i), map[string]any{"value": i})
	}

	resp := httpGet(t, "/api/v1/projections/sensor_state?limit=2&offset=0")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result ProjectionList
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 2, result.Limit)
	assert.Equal(t, 0, result.Offset)
	assert.Len(t, result.Projections, 2)
}
