package eventhandler

import (
	"context"

	"github.com/gofrs/uuid/v5"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// Projection represents a materialized view in the projections table.
type Projection struct {
	ProjectionID       uuid.UUID
	ProjectionType     string
	AggregateID        string
	State              []byte // JSONB
	LastEventID        uuid.UUID
	LastEventTimestamp string
}

// ProjectionRepository manages projection state.
type ProjectionRepository interface {
	// Upsert inserts or updates a projection, only if the event is newer.
	Upsert(ctx context.Context, projectionType, aggregateID string, state []byte, event *events.Envelope) error

	// Get retrieves a projection by type and aggregate ID.
	Get(ctx context.Context, projectionType, aggregateID string) (*Projection, error)
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
