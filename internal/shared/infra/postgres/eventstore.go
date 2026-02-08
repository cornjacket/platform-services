package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// EventStoreRepo implements outbox.EventStoreWriter using PostgreSQL.
type EventStoreRepo struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewEventStoreRepo creates a new EventStoreRepo.
func NewEventStoreRepo(pool *pgxpool.Pool, logger *slog.Logger) *EventStoreRepo {
	return &EventStoreRepo{
		pool:   pool,
		logger: logger.With("repository", "event_store"),
	}
}

// Insert adds an event to the event store.
// Returns an error if the event_id already exists (unique constraint).
func (r *EventStoreRepo) Insert(ctx context.Context, event *events.Envelope) error {
	query := `
		INSERT INTO event_store (event_id, event_type, aggregate_id, event_time, ingested_at, payload, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.pool.Exec(ctx, query,
		event.EventID,
		event.EventType,
		event.AggregateID,
		event.EventTime,
		event.IngestedAt,
		event.Payload,
		event.Metadata,
	)
	if err != nil {
		return fmt.Errorf("failed to insert into event_store: %w", err)
	}

	r.logger.Debug("event inserted into event_store",
		"event_id", event.EventID,
		"event_type", event.EventType,
	)

	return nil
}
