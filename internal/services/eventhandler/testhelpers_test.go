package eventhandler

import (
	"context"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// mockProjectionWriter implements ProjectionWriter for testing.
type mockProjectionWriter struct {
	WriteProjectionFn func(ctx context.Context, projType, aggregateID string, state []byte, event *events.Envelope) error
}

func (m *mockProjectionWriter) WriteProjection(ctx context.Context, projType, aggregateID string, state []byte, event *events.Envelope) error {
	return m.WriteProjectionFn(ctx, projType, aggregateID, state, event)
}

// mockEventHandler implements EventHandler for testing.
type mockEventHandler struct {
	HandleFn func(ctx context.Context, event *events.Envelope) error
}

func (m *mockEventHandler) Handle(ctx context.Context, event *events.Envelope) error {
	return m.HandleFn(ctx, event)
}
