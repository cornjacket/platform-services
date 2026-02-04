package ingestion

import (
	"context"

	"github.com/cornjacket/platform-services/internal/domain/events"
)

// OutboxRepository defines the interface for outbox operations.
// This interface is owned by the ingestion package.
// Infrastructure adapters (e.g., postgres) implement this interface.
type OutboxRepository interface {
	// Insert adds an event to the outbox table.
	// Returns the outbox entry ID on success.
	Insert(ctx context.Context, event *events.Envelope) error
}
