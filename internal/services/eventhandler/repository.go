package eventhandler

import (
	"context"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// ProjectionWriter writes projections to the store.
// This interface is satisfied by shared/projections.Store.
type ProjectionWriter interface {
	// WriteProjection inserts or updates a projection, only if the event is newer.
	WriteProjection(ctx context.Context, projType, aggregateID string, state []byte, event *events.Envelope) error
}

// EventHandler processes events and updates projections.
type EventHandler interface {
	// Handle processes a single event.
	Handle(ctx context.Context, event *events.Envelope) error
}

// EventConsumer consumes events from the message bus.
type EventConsumer interface {
	// Start begins consuming events and blocks until context is cancelled.
	Start(ctx context.Context) error

	// Close releases consumer resources.
	Close() error
}
