package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cornjacket/platform-services/internal/services/ingestion/worker"
	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// OutboxRepo implements ingestion.OutboxRepository using PostgreSQL.
type OutboxRepo struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewOutboxRepo creates a new OutboxRepo.
func NewOutboxRepo(pool *pgxpool.Pool, logger *slog.Logger) *OutboxRepo {
	return &OutboxRepo{
		pool:   pool,
		logger: logger.With("repository", "outbox"),
	}
}

// Insert adds an event to the outbox table.
func (r *OutboxRepo) Insert(ctx context.Context, event *events.Envelope) error {
	// Serialize the entire event envelope as the payload
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	query := `
		INSERT INTO outbox (outbox_id, event_payload, created_at)
		VALUES ($1, $2, $3)
	`

	_, err = r.pool.Exec(ctx, query, event.EventID, payload, event.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to insert into outbox: %w", err)
	}

	r.logger.Debug("event inserted into outbox",
		"event_id", event.EventID,
		"event_type", event.EventType,
	)

	return nil
}

// OutboxEntry represents a row in the outbox table (used by the processor).
type OutboxEntry struct {
	OutboxID   string
	Payload    *events.Envelope
	RetryCount int
}

// FetchPending retrieves unprocessed outbox entries.
// Used by the outbox processor.
func (r *OutboxRepo) FetchPending(ctx context.Context, limit int) ([]OutboxEntry, error) {
	query := `
		SELECT outbox_id, event_payload, retry_count
		FROM outbox
		ORDER BY created_at ASC
		LIMIT $1
	`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query outbox: %w", err)
	}
	defer rows.Close()

	var entries []OutboxEntry
	for rows.Next() {
		var entry OutboxEntry
		var payloadBytes []byte

		if err := rows.Scan(&entry.OutboxID, &payloadBytes, &entry.RetryCount); err != nil {
			return nil, fmt.Errorf("failed to scan outbox row: %w", err)
		}

		var envelope events.Envelope
		if err := json.Unmarshal(payloadBytes, &envelope); err != nil {
			return nil, fmt.Errorf("failed to unmarshal event payload: %w", err)
		}
		entry.Payload = &envelope

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating outbox rows: %w", err)
	}

	return entries, nil
}

// Delete removes a processed entry from the outbox.
func (r *OutboxRepo) Delete(ctx context.Context, outboxID string) error {
	query := `DELETE FROM outbox WHERE outbox_id = $1`

	result, err := r.pool.Exec(ctx, query, outboxID)
	if err != nil {
		return fmt.Errorf("failed to delete from outbox: %w", err)
	}

	if result.RowsAffected() == 0 {
		r.logger.Warn("outbox entry not found for deletion", "outbox_id", outboxID)
	}

	return nil
}

// IncrementRetry increments the retry count for an outbox entry.
func (r *OutboxRepo) IncrementRetry(ctx context.Context, outboxID string) error {
	query := `UPDATE outbox SET retry_count = retry_count + 1 WHERE outbox_id = $1`

	_, err := r.pool.Exec(ctx, query, outboxID)
	if err != nil {
		return fmt.Errorf("failed to increment retry count: %w", err)
	}

	return nil
}

// OutboxReaderAdapter adapts OutboxRepo to the worker.OutboxReader interface.
type OutboxReaderAdapter struct {
	repo *OutboxRepo
}

// NewOutboxReaderAdapter creates a new OutboxReaderAdapter.
func NewOutboxReaderAdapter(pool *pgxpool.Pool, logger *slog.Logger) *OutboxReaderAdapter {
	return &OutboxReaderAdapter{
		repo: NewOutboxRepo(pool, logger),
	}
}

// FetchPending implements worker.OutboxReader.
func (a *OutboxReaderAdapter) FetchPending(ctx context.Context, limit int) ([]worker.OutboxEntry, error) {
	entries, err := a.repo.FetchPending(ctx, limit)
	if err != nil {
		return nil, err
	}

	// Convert to worker package type
	result := make([]worker.OutboxEntry, len(entries))
	for i, e := range entries {
		result[i] = worker.OutboxEntry{
			OutboxID:   e.OutboxID,
			Payload:    e.Payload,
			RetryCount: e.RetryCount,
		}
	}
	return result, nil
}

// Delete implements worker.OutboxReader.
func (a *OutboxReaderAdapter) Delete(ctx context.Context, outboxID string) error {
	return a.repo.Delete(ctx, outboxID)
}

// IncrementRetry implements worker.OutboxReader.
func (a *OutboxReaderAdapter) IncrementRetry(ctx context.Context, outboxID string) error {
	return a.repo.IncrementRetry(ctx, outboxID)
}

// Ensure OutboxReaderAdapter implements worker.OutboxReader
var _ worker.OutboxReader = (*OutboxReaderAdapter)(nil)
