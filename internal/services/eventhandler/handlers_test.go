package eventhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

func newTestEnvelope(eventType string) *events.Envelope {
	envelope, _ := events.NewEnvelope(
		eventType, "device-001",
		json.RawMessage(`{"value": 72.5}`),
		events.Metadata{Source: "test"}, time.Now(),
	)
	return envelope
}

func TestDispatch_MatchedHandler(t *testing.T) {
	var handled bool
	mock := &mockEventHandler{
		HandleFn: func(ctx context.Context, event *events.Envelope) error {
			handled = true
			return nil
		},
	}

	registry := NewHandlerRegistry(slog.Default())
	registry.Register("sensor.", mock)

	err := registry.Dispatch(context.Background(), newTestEnvelope("sensor.reading"))
	require.NoError(t, err)
	assert.True(t, handled, "handler should be called for matching prefix")
}

func TestDispatch_NoHandler(t *testing.T) {
	registry := NewHandlerRegistry(slog.Default())

	err := registry.Dispatch(context.Background(), newTestEnvelope("unknown.event"))
	assert.NoError(t, err, "unmatched event should not error")
}

func TestDispatch_ErrorPropagation(t *testing.T) {
	mock := &mockEventHandler{
		HandleFn: func(ctx context.Context, event *events.Envelope) error {
			return fmt.Errorf("projection store unavailable")
		},
	}

	registry := NewHandlerRegistry(slog.Default())
	registry.Register("sensor.", mock)

	err := registry.Dispatch(context.Background(), newTestEnvelope("sensor.reading"))
	assert.Error(t, err)
}

func TestSensorHandler_Success(t *testing.T) {
	var capturedType, capturedAggID string
	mock := &mockProjectionWriter{
		WriteProjectionFn: func(ctx context.Context, projType, aggregateID string, state []byte, event *events.Envelope) error {
			capturedType = projType
			capturedAggID = aggregateID
			return nil
		},
	}

	handler := NewSensorHandler(mock, slog.Default())
	err := handler.Handle(context.Background(), newTestEnvelope("sensor.reading"))

	require.NoError(t, err)
	assert.Equal(t, "sensor_state", capturedType)
	assert.Equal(t, "device-001", capturedAggID)
}

func TestSensorHandler_StoreError(t *testing.T) {
	mock := &mockProjectionWriter{
		WriteProjectionFn: func(ctx context.Context, projType, aggregateID string, state []byte, event *events.Envelope) error {
			return fmt.Errorf("connection refused")
		},
	}

	handler := NewSensorHandler(mock, slog.Default())
	err := handler.Handle(context.Background(), newTestEnvelope("sensor.reading"))
	assert.Error(t, err)
}

func TestUserHandler_Success(t *testing.T) {
	var capturedType string
	mock := &mockProjectionWriter{
		WriteProjectionFn: func(ctx context.Context, projType, aggregateID string, state []byte, event *events.Envelope) error {
			capturedType = projType
			return nil
		},
	}

	handler := NewUserHandler(mock, slog.Default())
	err := handler.Handle(context.Background(), newTestEnvelope("user.login"))

	require.NoError(t, err)
	assert.Equal(t, "user_session", capturedType)
}

func TestUserHandler_StoreError(t *testing.T) {
	mock := &mockProjectionWriter{
		WriteProjectionFn: func(ctx context.Context, projType, aggregateID string, state []byte, event *events.Envelope) error {
			return fmt.Errorf("connection refused")
		},
	}

	handler := NewUserHandler(mock, slog.Default())
	err := handler.Handle(context.Background(), newTestEnvelope("user.login"))
	assert.Error(t, err)
}
