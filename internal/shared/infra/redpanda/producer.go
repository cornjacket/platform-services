package redpanda

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// Producer implements outbox.EventPublisher using Redpanda (Kafka-compatible).
type Producer struct {
	client *kgo.Client
	logger *slog.Logger
}

// NewProducer creates a new Redpanda producer.
func NewProducer(brokers []string, logger *slog.Logger) (*Producer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redpanda client: %w", err)
	}

	return &Producer{
		client: client,
		logger: logger.With("component", "redpanda-producer"),
	}, nil
}

// Publish sends an event to the specified topic.
func (p *Producer) Publish(ctx context.Context, topic string, event *events.Envelope) error {
	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	record := &kgo.Record{
		Topic: topic,
		Key:   []byte(event.AggregateID), // Partition by aggregate for ordering
		Value: value,
	}

	// Synchronous produce
	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("failed to publish to %s: %w", topic, err)
	}

	p.logger.Debug("event published to Redpanda",
		"topic", topic,
		"event_id", event.EventID,
		"event_type", event.EventType,
	)

	return nil
}

// Close closes the producer connection.
func (p *Producer) Close() {
	p.client.Close()
	p.logger.Info("Redpanda producer closed")
}
