package ingestion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

func TestHandleIngest_Success(t *testing.T) {
	var captured *events.Envelope
	mock := &mockOutboxRepository{
		InsertFn: func(ctx context.Context, event *events.Envelope) error {
			captured = event
			return nil
		},
	}
	service := NewService(mock, slog.Default())
	handler := NewHandler(service, slog.Default())

	body := `{"event_type":"sensor.reading","aggregate_id":"device-001","payload":{"value":72.5}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleIngest(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)

	var resp IngestResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "accepted", resp.Status)
	assert.NotEmpty(t, resp.EventID)
	require.NotNil(t, captured)
	assert.Equal(t, "sensor.reading", captured.EventType)
}

func TestHandleIngest_BadJSON(t *testing.T) {
	mock := &mockOutboxRepository{
		InsertFn: func(ctx context.Context, event *events.Envelope) error {
			t.Fatal("Insert should not be called for bad JSON")
			return nil
		},
	}
	service := NewService(mock, slog.Default())
	handler := NewHandler(service, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(`{not json`))
	w := httptest.NewRecorder()

	handler.HandleIngest(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleIngest_ValidationError(t *testing.T) {
	mock := &mockOutboxRepository{
		InsertFn: func(ctx context.Context, event *events.Envelope) error {
			t.Fatal("Insert should not be called for invalid request")
			return nil
		},
	}
	service := NewService(mock, slog.Default())
	handler := NewHandler(service, slog.Default())

	body := `{"aggregate_id":"device-001","payload":{"value":72.5}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleIngest(w, req)

	// Currently returns 500 (see TODO in handler.go). Important: not 202.
	assert.NotEqual(t, http.StatusAccepted, w.Code)
}

func TestHandleIngest_OutboxError(t *testing.T) {
	mock := &mockOutboxRepository{
		InsertFn: func(ctx context.Context, event *events.Envelope) error {
			return fmt.Errorf("connection refused")
		},
	}
	service := NewService(mock, slog.Default())
	handler := NewHandler(service, slog.Default())

	body := `{"event_type":"sensor.reading","aggregate_id":"device-001","payload":{"value":72.5}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	handler.HandleIngest(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleIngest_MethodNotAllowed(t *testing.T) {
	handler := NewHandler(NewService(nil, slog.Default()), slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	w := httptest.NewRecorder()

	handler.HandleIngest(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandleHealth(t *testing.T) {
	handler := NewHandler(NewService(nil, slog.Default()), slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.HandleHealth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "healthy", resp["status"])
}
