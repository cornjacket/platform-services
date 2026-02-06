package query

import (
	"context"
	"encoding/json"

	"github.com/gofrs/uuid/v5"
)

// Projection represents a projection record returned by the Query Service.
type Projection struct {
	ProjectionID       uuid.UUID       `json:"projection_id"`
	ProjectionType     string          `json:"projection_type"`
	AggregateID        string          `json:"aggregate_id"`
	State              json.RawMessage `json:"state"`
	LastEventID        uuid.UUID       `json:"last_event_id"`
	LastEventTimestamp string          `json:"last_event_timestamp"`
	UpdatedAt          string          `json:"updated_at"`
}

// ProjectionList represents a paginated list of projections.
type ProjectionList struct {
	Projections []Projection `json:"projections"`
	Total       int          `json:"total"`
	Limit       int          `json:"limit"`
	Offset      int          `json:"offset"`
}

// ProjectionRepository defines read operations for projections.
type ProjectionRepository interface {
	// Get retrieves a single projection by type and aggregate ID.
	Get(ctx context.Context, projectionType, aggregateID string) (*Projection, error)

	// List retrieves projections by type with pagination.
	// Returns the projections, total count, and any error.
	List(ctx context.Context, projectionType string, limit, offset int) ([]Projection, int, error)
}
