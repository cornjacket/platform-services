package worker

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
type OutboxReader interface {
	FetchPending(ctx context.Context, limit int) ([]OutboxEntry, error)
	Delete(ctx context.Context, outboxID string) error
	IncrementRetry(ctx context.Context, outboxID string) error
}

// EventStoreWriter writes events to the event store.
type EventStoreWriter interface {
	Insert(ctx context.Context, event *events.Envelope) error
}

// EventSubmitter submits events to the EventHandler for processing.
// This interface is satisfied by client/eventhandler.Client.
type EventSubmitter interface {
	SubmitEvent(ctx context.Context, event *events.Envelope) error
}
