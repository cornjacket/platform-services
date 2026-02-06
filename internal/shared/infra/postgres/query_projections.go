package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cornjacket/platform-services/internal/services/query"
)

// QueryProjectionRepo implements query.ProjectionRepository using PostgreSQL.
type QueryProjectionRepo struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewQueryProjectionRepo creates a new QueryProjectionRepo.
func NewQueryProjectionRepo(pool *pgxpool.Pool, logger *slog.Logger) *QueryProjectionRepo {
	return &QueryProjectionRepo{
		pool:   pool,
		logger: logger.With("repository", "query_projections"),
	}
}

// Get retrieves a single projection by type and aggregate ID.
func (r *QueryProjectionRepo) Get(ctx context.Context, projectionType, aggregateID string) (*query.Projection, error) {
	sql := `
		SELECT projection_id, projection_type, aggregate_id, state,
		       last_event_id, last_event_timestamp, updated_at
		FROM projections
		WHERE projection_type = $1 AND aggregate_id = $2
	`

	var p query.Projection
	var lastEventTimestamp, updatedAt string

	err := r.pool.QueryRow(ctx, sql, projectionType, aggregateID).Scan(
		&p.ProjectionID,
		&p.ProjectionType,
		&p.AggregateID,
		&p.State,
		&p.LastEventID,
		&lastEventTimestamp,
		&updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get projection: %w", err)
	}

	p.LastEventTimestamp = lastEventTimestamp
	p.UpdatedAt = updatedAt

	return &p, nil
}

// List retrieves projections by type with pagination.
func (r *QueryProjectionRepo) List(ctx context.Context, projectionType string, limit, offset int) ([]query.Projection, int, error) {
	// Get total count
	countSQL := `SELECT COUNT(*) FROM projections WHERE projection_type = $1`
	var total int
	if err := r.pool.QueryRow(ctx, countSQL, projectionType).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count projections: %w", err)
	}

	// Get projections with pagination
	listSQL := `
		SELECT projection_id, projection_type, aggregate_id, state,
		       last_event_id, last_event_timestamp, updated_at
		FROM projections
		WHERE projection_type = $1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := r.pool.Query(ctx, listSQL, projectionType, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list projections: %w", err)
	}
	defer rows.Close()

	var projections []query.Projection
	for rows.Next() {
		var p query.Projection
		var lastEventTimestamp, updatedAt string

		if err := rows.Scan(
			&p.ProjectionID,
			&p.ProjectionType,
			&p.AggregateID,
			&p.State,
			&p.LastEventID,
			&lastEventTimestamp,
			&updatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan projection: %w", err)
		}

		p.LastEventTimestamp = lastEventTimestamp
		p.UpdatedAt = updatedAt
		projections = append(projections, p)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating projections: %w", err)
	}

	// Return empty slice instead of nil if no results
	if projections == nil {
		projections = []query.Projection{}
	}

	return projections, total, nil
}

// Ensure QueryProjectionRepo implements query.ProjectionRepository
var _ query.ProjectionRepository = (*QueryProjectionRepo)(nil)
