package ingestion

import (
	"context"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// mockOutboxRepository implements OutboxRepository for testing.
type mockOutboxRepository struct {
	InsertFn func(ctx context.Context, event *events.Envelope) error
}

func (m *mockOutboxRepository) Insert(ctx context.Context, event *events.Envelope) error {
	return m.InsertFn(ctx, event)
}
