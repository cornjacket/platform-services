package query

import (
	"context"
	"encoding/json"

	"github.com/gofrs/uuid/v5"

	"github.com/cornjacket/platform-services/internal/shared/projections"
)

// Projection represents a projection record returned by the Query Service.
// This is the API response format with string timestamps for JSON serialization.
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

// ProjectionReader reads projections from the store.
// This interface is satisfied by shared/projections.Store.
type ProjectionReader interface {
	// GetProjection retrieves a single projection by type and aggregate ID.
	GetProjection(ctx context.Context, projType, aggregateID string) (*projections.Projection, error)

	// ListProjections retrieves projections by type with pagination.
	ListProjections(ctx context.Context, projType string, limit, offset int) ([]projections.Projection, int, error)
}

// fromStoreProjection converts a shared projections.Projection to query.Projection
func fromStoreProjection(p *projections.Projection) *Projection {
	return &Projection{
		ProjectionID:       p.ProjectionID,
		ProjectionType:     p.ProjectionType,
		AggregateID:        p.AggregateID,
		State:              p.State,
		LastEventID:        p.LastEventID,
		LastEventTimestamp: p.LastEventTimestamp.Format("2006-01-02T15:04:05.000Z"),
		UpdatedAt:          p.UpdatedAt.Format("2006-01-02T15:04:05.000Z"),
	}
}

// fromStoreProjections converts a slice of shared projections.Projection to query.Projection
func fromStoreProjections(ps []projections.Projection) []Projection {
	result := make([]Projection, len(ps))
	for i, p := range ps {
		result[i] = *fromStoreProjection(&p)
	}
	return result
}
