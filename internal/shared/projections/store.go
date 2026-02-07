package projections

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// Projection represents a materialized view in the projections table.
type Projection struct {
	ProjectionID       uuid.UUID       `json:"projection_id"`
	ProjectionType     string          `json:"projection_type"`
	AggregateID        string          `json:"aggregate_id"`
	State              json.RawMessage `json:"state"`
	LastEventID        uuid.UUID       `json:"last_event_id"`
	LastEventTimestamp time.Time       `json:"last_event_timestamp"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

// Store provides read and write operations for projections.
// This interface is used by both EventHandler (write) and Query Service (read).
type Store interface {
	// WriteProjection inserts or updates a projection, only if the event is newer.
	WriteProjection(ctx context.Context, projType, aggregateID string, state []byte, event *events.Envelope) error

	// GetProjection retrieves a single projection by type and aggregate ID.
	GetProjection(ctx context.Context, projType, aggregateID string) (*Projection, error)

	// ListProjections retrieves projections by type with pagination.
	// Returns the projections, total count, and any error.
	ListProjections(ctx context.Context, projType string, limit, offset int) ([]Projection, int, error)
}
