package projections

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// PostgresStore implements Store using PostgreSQL.
type PostgresStore struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewPostgresStore creates a new PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool, logger *slog.Logger) *PostgresStore {
	return &PostgresStore{
		pool:   pool,
		logger: logger.With("store", "projections"),
	}
}

// WriteProjection inserts or updates a projection, only if the event is newer.
func (s *PostgresStore) WriteProjection(ctx context.Context, projType, aggregateID string, state []byte, event *events.Envelope) error {
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

	result, err := s.pool.Exec(ctx, query,
		projType,
		aggregateID,
		state,
		event.EventID,
		event.EventTime,
	)
	if err != nil {
		return fmt.Errorf("failed to write projection: %w", err)
	}

	if result.RowsAffected() == 0 {
		s.logger.Debug("projection not updated (event not newer)",
			"projection_type", projType,
			"aggregate_id", aggregateID,
			"event_id", event.EventID,
		)
	}

	return nil
}

// GetProjection retrieves a single projection by type and aggregate ID.
func (s *PostgresStore) GetProjection(ctx context.Context, projType, aggregateID string) (*Projection, error) {
	query := `
		SELECT projection_id, projection_type, aggregate_id, state,
		       last_event_id, last_event_timestamp, updated_at
		FROM projections
		WHERE projection_type = $1 AND aggregate_id = $2
	`

	var p Projection
	var projID, lastEventID uuid.UUID
	var lastEventTimestamp, updatedAt time.Time

	err := s.pool.QueryRow(ctx, query, projType, aggregateID).Scan(
		&projID,
		&p.ProjectionType,
		&p.AggregateID,
		&p.State,
		&lastEventID,
		&lastEventTimestamp,
		&updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get projection: %w", err)
	}

	p.ProjectionID = projID
	p.LastEventID = lastEventID
	p.LastEventTimestamp = lastEventTimestamp
	p.UpdatedAt = updatedAt

	return &p, nil
}

// ListProjections retrieves projections by type with pagination.
func (s *PostgresStore) ListProjections(ctx context.Context, projType string, limit, offset int) ([]Projection, int, error) {
	// Get total count
	countSQL := `SELECT COUNT(*) FROM projections WHERE projection_type = $1`
	var total int
	if err := s.pool.QueryRow(ctx, countSQL, projType).Scan(&total); err != nil {
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

	rows, err := s.pool.Query(ctx, listSQL, projType, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list projections: %w", err)
	}
	defer rows.Close()

	var projections []Projection
	for rows.Next() {
		var p Projection
		var projID, lastEventID uuid.UUID
		var lastEventTimestamp, updatedAt time.Time

		if err := rows.Scan(
			&projID,
			&p.ProjectionType,
			&p.AggregateID,
			&p.State,
			&lastEventID,
			&lastEventTimestamp,
			&updatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan projection: %w", err)
		}

		p.ProjectionID = projID
		p.LastEventID = lastEventID
		p.LastEventTimestamp = lastEventTimestamp
		p.UpdatedAt = updatedAt
		projections = append(projections, p)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating projections: %w", err)
	}

	// Return empty slice instead of nil if no results
	if projections == nil {
		projections = []Projection{}
	}

	return projections, total, nil
}

// Ensure PostgresStore implements Store
var _ Store = (*PostgresStore)(nil)
