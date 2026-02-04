package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
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

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

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

// NewEnvelope creates a new event envelope with a generated ID and current timestamp.
func NewEnvelope(eventType, aggregateID string, payload any, metadata Metadata) (*Envelope, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Envelope{
		EventID:     uuid.New(),
		EventType:   eventType,
		AggregateID: aggregateID,
		Timestamp:   time.Now().UTC(),
		Payload:     payloadBytes,
		Metadata:    metadata,
	}, nil
}

// ParsePayload unmarshals the payload into the provided type.
func (e *Envelope) ParsePayload(v any) error {
	return json.Unmarshal(e.Payload, v)
}
