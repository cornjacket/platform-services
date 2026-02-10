package worker

import (
	"context"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// mockOutboxReader implements OutboxReader for testing.
type mockOutboxReader struct {
	FetchPendingFn   func(ctx context.Context, limit int) ([]OutboxEntry, error)
	DeleteFn         func(ctx context.Context, outboxID string) error
	IncrementRetryFn func(ctx context.Context, outboxID string) error
}

func (m *mockOutboxReader) FetchPending(ctx context.Context, limit int) ([]OutboxEntry, error) {
	return m.FetchPendingFn(ctx, limit)
}

func (m *mockOutboxReader) Delete(ctx context.Context, outboxID string) error {
	return m.DeleteFn(ctx, outboxID)
}

func (m *mockOutboxReader) IncrementRetry(ctx context.Context, outboxID string) error {
	return m.IncrementRetryFn(ctx, outboxID)
}

// mockEventStoreWriter implements EventStoreWriter for testing.
type mockEventStoreWriter struct {
	InsertFn func(ctx context.Context, event *events.Envelope) error
}

func (m *mockEventStoreWriter) Insert(ctx context.Context, event *events.Envelope) error {
	return m.InsertFn(ctx, event)
}

// mockEventSubmitter implements EventSubmitter for testing.
type mockEventSubmitter struct {
	SubmitEventFn func(ctx context.Context, event *events.Envelope) error
}

func (m *mockEventSubmitter) SubmitEvent(ctx context.Context, event *events.Envelope) error {
	return m.SubmitEventFn(ctx, event)
}
