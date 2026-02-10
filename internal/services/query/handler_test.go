package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gofrs/uuid/v5"

	"github.com/cornjacket/platform-services/internal/shared/projections"
)

func newTestProjection() *projections.Projection {
	return &projections.Projection{
		ProjectionID:       uuid.Must(uuid.NewV7()),
		ProjectionType:     "sensor_state",
		AggregateID:        "device-001",
		State:              json.RawMessage(`{"temperature": 72.5}`),
		LastEventID:        uuid.Must(uuid.NewV7()),
		LastEventTimestamp: time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC),
	}
}

func TestHandleGetProjection_Success(t *testing.T) {
	mock := &mockProjectionReader{
		GetProjectionFn: func(ctx context.Context, projType, aggregateID string) (*projections.Projection, error) {
			return newTestProjection(), nil
		},
	}
	service := NewService(mock, slog.Default())
	handler := NewHandler(service, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projections/sensor_state/device-001", nil)
	w := httptest.NewRecorder()

	handler.HandleGetProjection(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp Projection
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "device-001", resp.AggregateID)
}

func TestHandleGetProjection_NotFound(t *testing.T) {
	mock := &mockProjectionReader{
		GetProjectionFn: func(ctx context.Context, projType, aggregateID string) (*projections.Projection, error) {
			return nil, fmt.Errorf("no rows in result set")
		},
	}
	service := NewService(mock, slog.Default())
	handler := NewHandler(service, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projections/sensor_state/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.HandleGetProjection(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleGetProjection_InvalidType(t *testing.T) {
	mock := &mockProjectionReader{
		GetProjectionFn: func(ctx context.Context, projType, aggregateID string) (*projections.Projection, error) {
			t.Fatal("store should not be called for invalid type")
			return nil, nil
		},
	}
	service := NewService(mock, slog.Default())
	handler := NewHandler(service, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projections/invalid_type/device-001", nil)
	w := httptest.NewRecorder()

	handler.HandleGetProjection(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleGetProjection_BadPath(t *testing.T) {
	handler := NewHandler(NewService(nil, slog.Default()), slog.Default())

	tests := []struct {
		name string
		path string
	}{
		{"no segments", "/api/v1/projections/"},
		{"one segment", "/api/v1/projections/sensor_state"},
		{"three segments", "/api/v1/projections/sensor_state/device-001/extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()

			handler.HandleGetProjection(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestHandleGetProjection_MethodNotAllowed(t *testing.T) {
	handler := NewHandler(NewService(nil, slog.Default()), slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projections/sensor_state/device-001", nil)
	w := httptest.NewRecorder()

	handler.HandleGetProjection(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandleListProjections_Success(t *testing.T) {
	mock := &mockProjectionReader{
		ListProjectionsFn: func(ctx context.Context, projType string, limit, offset int) ([]projections.Projection, int, error) {
			p := newTestProjection()
			return []projections.Projection{*p}, 1, nil
		},
	}
	service := NewService(mock, slog.Default())
	handler := NewHandler(service, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projections/sensor_state?limit=10&offset=0", nil)
	w := httptest.NewRecorder()

	handler.HandleListProjections(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ProjectionList
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 1, resp.Total)
}

func TestHandleListProjections_PaginationParams(t *testing.T) {
	var capturedLimit, capturedOffset int
	mock := &mockProjectionReader{
		ListProjectionsFn: func(ctx context.Context, projType string, limit, offset int) ([]projections.Projection, int, error) {
			capturedLimit = limit
			capturedOffset = offset
			return nil, 0, nil
		},
	}
	service := NewService(mock, slog.Default())
	handler := NewHandler(service, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projections/sensor_state?limit=50&offset=25", nil)
	w := httptest.NewRecorder()

	handler.HandleListProjections(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 50, capturedLimit)
	assert.Equal(t, 25, capturedOffset)
}

func TestHandleListProjections_MethodNotAllowed(t *testing.T) {
	handler := NewHandler(NewService(nil, slog.Default()), slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projections/sensor_state", nil)
	w := httptest.NewRecorder()

	handler.HandleListProjections(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandleHealth_Query(t *testing.T) {
	handler := NewHandler(NewService(nil, slog.Default()), slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.HandleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "healthy", resp["status"])
}
