package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// Service handles event ingestion business logic.
type Service struct {
	outbox OutboxRepository
	logger *slog.Logger
}

// NewService creates a new ingestion service.
func NewService(outbox OutboxRepository, logger *slog.Logger) *Service {
	return &Service{
		outbox: outbox,
		logger: logger.With("service", "ingestion"),
	}
}

// IngestRequest represents an incoming event ingestion request.
type IngestRequest struct {
	EventType   string          `json:"event_type"`
	AggregateID string          `json:"aggregate_id"`
	Payload     json.RawMessage `json:"payload"`
	TraceID     string          `json:"trace_id,omitempty"`
}

// IngestResponse is returned after successful ingestion.
type IngestResponse struct {
	EventID string `json:"event_id"`
	Status  string `json:"status"`
}

// Ingest validates and writes an event to the outbox.
func (s *Service) Ingest(ctx context.Context, req *IngestRequest) (*IngestResponse, error) {
	// Validate request
	if err := s.validate(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Create event envelope
	envelope, err := events.NewEnvelope(
		req.EventType,
		req.AggregateID,
		req.Payload,
		events.Metadata{
			TraceID:       req.TraceID,
			Source:        "ingestion-api",
			SchemaVersion: 1,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create event envelope: %w", err)
	}

	// Write to outbox
	if err := s.outbox.Insert(ctx, envelope); err != nil {
		s.logger.Error("failed to insert into outbox",
			"event_id", envelope.EventID,
			"event_type", envelope.EventType,
			"error", err,
		)
		return nil, fmt.Errorf("failed to write to outbox: %w", err)
	}

	s.logger.Info("event ingested",
		"event_id", envelope.EventID,
		"event_type", envelope.EventType,
		"aggregate_id", envelope.AggregateID,
	)

	return &IngestResponse{
		EventID: envelope.EventID.String(),
		Status:  "accepted",
	}, nil
}

func (s *Service) validate(req *IngestRequest) error {
	if req.EventType == "" {
		return fmt.Errorf("event_type is required")
	}
	if req.AggregateID == "" {
		return fmt.Errorf("aggregate_id is required")
	}
	if len(req.Payload) == 0 {
		return fmt.Errorf("payload is required")
	}

	// Validate payload is valid JSON
	var js json.RawMessage
	if err := json.Unmarshal(req.Payload, &js); err != nil {
		return fmt.Errorf("payload must be valid JSON: %w", err)
	}

	return nil
}
