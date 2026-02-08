package events

import (
	"encoding/json"
	"time"

	"github.com/cornjacket/platform-services/internal/shared/domain/clock"
	"github.com/gofrs/uuid/v5"
)

// Envelope is the common structure for all events in the system.
// This same structure is used in the outbox table, event store, and Redpanda messages.
type Envelope struct {
	// EventID is the unique identifier for this event
	EventID uuid.UUID `json:"event_id"`

	// EventType is the discriminator (e.g., "sensor.reading", "user.action")
	EventType string `json:"event_type"`

	// AggregateID groups related events (e.g., device ID, session ID)
	AggregateID string `json:"aggregate_id"`

	// EventTime is when the event occurred (from the caller/producer)
	EventTime time.Time `json:"event_time"`

	// IngestedAt is when the platform received the event (set by platform clock)
	IngestedAt time.Time `json:"ingested_at"`

	// Payload contains the event-specific data
	Payload json.RawMessage `json:"payload"`

	// Metadata contains trace IDs, source info, schema version, etc.
	Metadata Metadata `json:"metadata"`
}

// Metadata contains contextual information about the event.
type Metadata struct {
	// TraceID for distributed tracing (optional)
	TraceID string `json:"trace_id,omitempty"`

	// Source identifies where the event originated
	Source string `json:"source,omitempty"`

	// SchemaVersion for payload evolution
	SchemaVersion int `json:"schema_version"`
}

// NewEnvelope creates a new event envelope.
// eventTime is provided by the caller (when the event occurred).
// IngestedAt is set automatically by the platform clock.
func NewEnvelope(eventType, aggregateID string, payload any, metadata Metadata, eventTime time.Time) (*Envelope, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Envelope{
		EventID:     uuid.Must(uuid.NewV7()),
		EventType:   eventType,
		AggregateID: aggregateID,
		EventTime:   eventTime,
		IngestedAt:  clock.Now(),
		Payload:     payloadBytes,
		Metadata:    metadata,
	}, nil
}

// ParsePayload unmarshals the payload into the provided type.
func (e *Envelope) ParsePayload(v any) error {
	return json.Unmarshal(e.Payload, v)
}
