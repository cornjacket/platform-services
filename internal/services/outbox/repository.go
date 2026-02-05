package outbox

import (
	"context"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// OutboxEntry represents a row in the outbox table.
type OutboxEntry struct {
	OutboxID   string
	Payload    *events.Envelope
	RetryCount int
}

// OutboxReader reads and manages outbox entries.
// This interface is owned by the outbox package.
// Infrastructure adapters (e.g., postgres) implement this interface.
type OutboxReader interface {
	FetchPending(ctx context.Context, limit int) ([]OutboxEntry, error)
	Delete(ctx context.Context, outboxID string) error
	IncrementRetry(ctx context.Context, outboxID string) error
}

// EventStoreWriter writes events to the event store.
type EventStoreWriter interface {
	Insert(ctx context.Context, event *events.Envelope) error
}

// EventPublisher publishes events to the message bus.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, event *events.Envelope) error
}
