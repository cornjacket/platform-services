package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cornjacket/platform-services/internal/services/eventhandler"
	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// ProjectionRepo implements eventhandler.ProjectionRepository using PostgreSQL.
type ProjectionRepo struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewProjectionRepo creates a new ProjectionRepo.
func NewProjectionRepo(pool *pgxpool.Pool, logger *slog.Logger) *ProjectionRepo {
	return &ProjectionRepo{
		pool:   pool,
		logger: logger.With("repository", "projections"),
	}
}

// Upsert inserts or updates a projection, only if the event is newer.
func (r *ProjectionRepo) Upsert(ctx context.Context, projectionType, aggregateID string, state []byte, event *events.Envelope) error {
	// Use ON CONFLICT to handle upsert
	// Only update if the incoming event is newer than the stored one
	query := `
		INSERT INTO projections (projection_type, aggregate_id, state, last_event_id, last_event_timestamp, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (projection_type, aggregate_id) DO UPDATE
		SET state = EXCLUDED.state,
		    last_event_id = EXCLUDED.last_event_id,
		    last_event_timestamp = EXCLUDED.last_event_timestamp,
		    updated_at = NOW()
		WHERE projections.last_event_timestamp < EXCLUDED.last_event_timestamp
		   OR (projections.last_event_timestamp = EXCLUDED.last_event_timestamp
		       AND projections.last_event_id < EXCLUDED.last_event_id)
	`

	result, err := r.pool.Exec(ctx, query,
		projectionType,
		aggregateID,
		state,
		event.EventID,
		event.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert projection: %w", err)
	}

	if result.RowsAffected() == 0 {
		r.logger.Debug("projection not updated (event not newer)",
			"projection_type", projectionType,
			"aggregate_id", aggregateID,
			"event_id", event.EventID,
		)
	}

	return nil
}

// Get retrieves a projection by type and aggregate ID.
func (r *ProjectionRepo) Get(ctx context.Context, projectionType, aggregateID string) (*eventhandler.Projection, error) {
	query := `
		SELECT projection_id, projection_type, aggregate_id, state, last_event_id, last_event_timestamp
		FROM projections
		WHERE projection_type = $1 AND aggregate_id = $2
	`

	var projection eventhandler.Projection
	var lastEventTimestamp string

	err := r.pool.QueryRow(ctx, query, projectionType, aggregateID).Scan(
		&projection.ProjectionID,
		&projection.ProjectionType,
		&projection.AggregateID,
		&projection.State,
		&projection.LastEventID,
		&lastEventTimestamp,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get projection: %w", err)
	}

	projection.LastEventTimestamp = lastEventTimestamp

	return &projection, nil
}

// Ensure ProjectionRepo implements eventhandler.ProjectionRepository
var _ eventhandler.ProjectionRepository = (*ProjectionRepo)(nil)
