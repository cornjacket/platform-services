package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
	"github.com/jackc/pgx/v5/pgconn"
)

func newTestEntry() OutboxEntry {
	envelope, _ := events.NewEnvelope(
		"sensor.reading", "device-001",
		json.RawMessage(`{"value": 72.5}`),
		events.Metadata{Source: "test"}, time.Now(),
	)
	return OutboxEntry{OutboxID: "outbox-001", Payload: envelope, RetryCount: 0}
}

func TestProcessEntry_Success(t *testing.T) {
	var eventStoreInserted, submitted, deleted bool

	outbox := &mockOutboxReader{
		DeleteFn: func(ctx context.Context, outboxID string) error {
			deleted = true
			assert.Equal(t, "outbox-001", outboxID)
			return nil
		},
		IncrementRetryFn: func(ctx context.Context, outboxID string) error {
			t.Fatal("IncrementRetry should not be called on success")
			return nil
		},
	}
	eventStore := &mockEventStoreWriter{
		InsertFn: func(ctx context.Context, event *events.Envelope) error {
			eventStoreInserted = true
			return nil
		},
	}
	submitter := &mockEventSubmitter{
		SubmitEventFn: func(ctx context.Context, event *events.Envelope) error {
			submitted = true
			return nil
		},
	}

	p := &Processor{outbox: outbox, eventStore: eventStore, submitter: submitter, config: ProcessorConfig{MaxRetries: 5}, logger: slog.Default()}
	p.processEntry(context.Background(), slog.Default(), newTestEntry())

	assert.True(t, eventStoreInserted, "event store Insert should be called")
	assert.True(t, submitted, "submitter SubmitEvent should be called")
	assert.True(t, deleted, "outbox Delete should be called")
}

func TestProcessEntry_MaxRetriesExceeded(t *testing.T) {
	outbox := &mockOutboxReader{
		DeleteFn: func(ctx context.Context, outboxID string) error {
			t.Fatal("Delete should not be called when max retries exceeded")
			return nil
		},
		IncrementRetryFn: func(ctx context.Context, outboxID string) error {
			t.Fatal("IncrementRetry should not be called when max retries exceeded")
			return nil
		},
	}
	eventStore := &mockEventStoreWriter{
		InsertFn: func(ctx context.Context, event *events.Envelope) error {
			t.Fatal("Insert should not be called when max retries exceeded")
			return nil
		},
	}
	submitter := &mockEventSubmitter{
		SubmitEventFn: func(ctx context.Context, event *events.Envelope) error {
			t.Fatal("SubmitEvent should not be called when max retries exceeded")
			return nil
		},
	}

	p := &Processor{outbox: outbox, eventStore: eventStore, submitter: submitter, config: ProcessorConfig{MaxRetries: 5}, logger: slog.Default()}

	entry := newTestEntry()
	entry.RetryCount = 5
	p.processEntry(context.Background(), slog.Default(), entry)
}

func TestProcessEntry_DuplicateEvent(t *testing.T) {
	var submitted, deleted bool

	outbox := &mockOutboxReader{
		DeleteFn: func(ctx context.Context, outboxID string) error {
			deleted = true
			return nil
		},
		IncrementRetryFn: func(ctx context.Context, outboxID string) error {
			t.Fatal("IncrementRetry should not be called for duplicate")
			return nil
		},
	}
	eventStore := &mockEventStoreWriter{
		InsertFn: func(ctx context.Context, event *events.Envelope) error {
			return &pgconn.PgError{Code: "23505", Message: "unique_violation"}
		},
	}
	submitter := &mockEventSubmitter{
		SubmitEventFn: func(ctx context.Context, event *events.Envelope) error {
			submitted = true
			return nil
		},
	}

	p := &Processor{outbox: outbox, eventStore: eventStore, submitter: submitter, config: ProcessorConfig{MaxRetries: 5}, logger: slog.Default()}
	p.processEntry(context.Background(), slog.Default(), newTestEntry())

	assert.True(t, submitted, "submitter should still be called after duplicate")
	assert.True(t, deleted, "outbox Delete should still be called after duplicate")
}

func TestProcessEntry_SubmitError(t *testing.T) {
	var retried bool

	outbox := &mockOutboxReader{
		DeleteFn: func(ctx context.Context, outboxID string) error {
			t.Fatal("Delete should not be called when submit fails")
			return nil
		},
		IncrementRetryFn: func(ctx context.Context, outboxID string) error {
			retried = true
			return nil
		},
	}
	eventStore := &mockEventStoreWriter{
		InsertFn: func(ctx context.Context, event *events.Envelope) error { return nil },
	}
	submitter := &mockEventSubmitter{
		SubmitEventFn: func(ctx context.Context, event *events.Envelope) error {
			return fmt.Errorf("kafka unavailable")
		},
	}

	p := &Processor{outbox: outbox, eventStore: eventStore, submitter: submitter, config: ProcessorConfig{MaxRetries: 5}, logger: slog.Default()}
	p.processEntry(context.Background(), slog.Default(), newTestEntry())

	assert.True(t, retried, "IncrementRetry should be called when submit fails")
}

func TestProcessEntry_DeleteError(t *testing.T) {
	outbox := &mockOutboxReader{
		DeleteFn: func(ctx context.Context, outboxID string) error {
			return fmt.Errorf("connection lost")
		},
		IncrementRetryFn: func(ctx context.Context, outboxID string) error {
			t.Fatal("IncrementRetry should not be called on delete error")
			return nil
		},
	}
	eventStore := &mockEventStoreWriter{
		InsertFn: func(ctx context.Context, event *events.Envelope) error { return nil },
	}
	submitter := &mockEventSubmitter{
		SubmitEventFn: func(ctx context.Context, event *events.Envelope) error { return nil },
	}

	p := &Processor{outbox: outbox, eventStore: eventStore, submitter: submitter, config: ProcessorConfig{MaxRetries: 5}, logger: slog.Default()}
	p.processEntry(context.Background(), slog.Default(), newTestEntry())
	// Delete error logged but not retried — idempotency handles reprocessing
}

func TestIsDuplicateError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"unique violation", &pgconn.PgError{Code: "23505"}, true},
		{"other pg error", &pgconn.PgError{Code: "23503"}, false},
		{"non-pg error", fmt.Errorf("connection refused"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isDuplicateError(tt.err))
		})
	}
}
