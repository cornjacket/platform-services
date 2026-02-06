package eventhandler

import (
	"context"
	"log/slog"
	"strings"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// HandlerRegistry dispatches events to appropriate handlers based on event_type prefix.
type HandlerRegistry struct {
	handlers map[string]EventHandler
	logger   *slog.Logger
}

// NewHandlerRegistry creates a new handler registry.
func NewHandlerRegistry(logger *slog.Logger) *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]EventHandler),
		logger:   logger.With("component", "handler-registry"),
	}
}

// Register adds a handler for events with the given prefix.
func (r *HandlerRegistry) Register(prefix string, handler EventHandler) {
	r.handlers[prefix] = handler
	r.logger.Info("registered handler", "prefix", prefix)
}

// Dispatch routes an event to the appropriate handler.
func (r *HandlerRegistry) Dispatch(ctx context.Context, event *events.Envelope) error {
	for prefix, handler := range r.handlers {
		if strings.HasPrefix(event.EventType, prefix) {
			return handler.Handle(ctx, event)
		}
	}
	// No handler registered - log and skip (not an error)
	r.logger.Debug("no handler for event type", "event_type", event.EventType)
	return nil
}

// SensorHandler processes sensor.* events.
type SensorHandler struct {
	repo   ProjectionRepository
	logger *slog.Logger
}

// NewSensorHandler creates a new sensor event handler.
func NewSensorHandler(repo ProjectionRepository, logger *slog.Logger) *SensorHandler {
	return &SensorHandler{
		repo:   repo,
		logger: logger.With("handler", "sensor"),
	}
}

// Handle processes a sensor event and updates the sensor_state projection.
func (h *SensorHandler) Handle(ctx context.Context, event *events.Envelope) error {
	err := h.repo.Upsert(ctx, "sensor_state", event.AggregateID, event.Payload, event)
	if err != nil {
		h.logger.Error("failed to update sensor_state projection",
			"event_id", event.EventID,
			"aggregate_id", event.AggregateID,
			"error", err,
		)
		return err
	}

	h.logger.Debug("updated sensor_state projection",
		"event_id", event.EventID,
		"aggregate_id", event.AggregateID,
	)
	return nil
}

// UserHandler processes user.* events.
type UserHandler struct {
	repo   ProjectionRepository
	logger *slog.Logger
}

// NewUserHandler creates a new user event handler.
func NewUserHandler(repo ProjectionRepository, logger *slog.Logger) *UserHandler {
	return &UserHandler{
		repo:   repo,
		logger: logger.With("handler", "user"),
	}
}

// Handle processes a user event and updates the user_session projection.
func (h *UserHandler) Handle(ctx context.Context, event *events.Envelope) error {
	err := h.repo.Upsert(ctx, "user_session", event.AggregateID, event.Payload, event)
	if err != nil {
		h.logger.Error("failed to update user_session projection",
			"event_id", event.EventID,
			"aggregate_id", event.AggregateID,
			"error", err,
		)
		return err
	}

	h.logger.Debug("updated user_session projection",
		"event_id", event.EventID,
		"aggregate_id", event.AggregateID,
	)
	return nil
}
