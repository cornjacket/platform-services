package eventhandler

import (
	"context"
	"log/slog"
	"strings"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// EventPublisher publishes events to the message bus.
type EventPublisher interface {
	Publish(ctx context.Context, topic string, event *events.Envelope) error
}

// Client provides methods for submitting events to the EventHandler service.
// It wraps the underlying message bus (Redpanda) to provide a service-level abstraction.
type Client struct {
	publisher EventPublisher
	logger    *slog.Logger
}

// New creates a new EventHandler client.
func New(publisher EventPublisher, logger *slog.Logger) *Client {
	return &Client{
		publisher: publisher,
		logger:    logger.With("client", "eventhandler"),
	}
}

// SubmitEvent sends an event to the EventHandler for processing.
// The event will be routed to the appropriate topic based on its type.
func (c *Client) SubmitEvent(ctx context.Context, event *events.Envelope) error {
	topic := topicFromEventType(event.EventType)

	if err := c.publisher.Publish(ctx, topic, event); err != nil {
		c.logger.Error("failed to submit event",
			"event_id", event.EventID,
			"event_type", event.EventType,
			"topic", topic,
			"error", err,
		)
		return err
	}

	c.logger.Debug("event submitted to EventHandler",
		"event_id", event.EventID,
		"event_type", event.EventType,
		"topic", topic,
	)

	return nil
}

// topicFromEventType derives the Redpanda topic from the event type.
func topicFromEventType(eventType string) string {
	switch {
	case strings.HasPrefix(eventType, "sensor."):
		return "sensor-events"
	case strings.HasPrefix(eventType, "user."):
		return "user-actions"
	default:
		return "system-events"
	}
}
