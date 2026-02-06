package eventhandler

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// ConsumerConfig holds configuration for the event consumer.
type ConsumerConfig struct {
	Brokers      []string
	GroupID      string
	Topics       []string
	PollTimeout  time.Duration
}

// Consumer consumes events from Redpanda and dispatches to handlers.
type Consumer struct {
	client   *kgo.Client
	registry *HandlerRegistry
	config   ConsumerConfig
	logger   *slog.Logger
}

// NewConsumer creates a new event consumer.
func NewConsumer(
	registry *HandlerRegistry,
	config ConsumerConfig,
	logger *slog.Logger,
) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(config.Brokers...),
		kgo.ConsumerGroup(config.GroupID),
		kgo.ConsumeTopics(config.Topics...),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, err
	}

	return &Consumer{
		client:   client,
		registry: registry,
		config:   config,
		logger:   logger.With("component", "event-consumer"),
	}, nil
}

// Start begins consuming events and blocks until context is cancelled.
func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("starting event consumer",
		"group_id", c.config.GroupID,
		"topics", c.config.Topics,
	)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("event consumer stopping")
			return nil
		default:
		}

		fetches := c.client.PollFetches(ctx)
		if fetches.IsClientClosed() {
			return nil
		}

		if errs := fetches.Errors(); len(errs) > 0 {
			for _, err := range errs {
				c.logger.Error("fetch error",
					"topic", err.Topic,
					"partition", err.Partition,
					"error", err.Err,
				)
			}
			continue
		}

		fetches.EachRecord(func(record *kgo.Record) {
			c.processRecord(ctx, record)
		})

		// Commit offsets after processing batch
		if err := c.client.CommitUncommittedOffsets(ctx); err != nil {
			c.logger.Error("failed to commit offsets", "error", err)
		}
	}
}

// processRecord processes a single Kafka record.
func (c *Consumer) processRecord(ctx context.Context, record *kgo.Record) {
	logger := c.logger.With(
		"topic", record.Topic,
		"partition", record.Partition,
		"offset", record.Offset,
	)

	// Deserialize event
	var event events.Envelope
	if err := json.Unmarshal(record.Value, &event); err != nil {
		logger.Error("failed to deserialize event", "error", err)
		return
	}

	logger = logger.With(
		"event_id", event.EventID,
		"event_type", event.EventType,
		"aggregate_id", event.AggregateID,
	)

	// Dispatch to handler
	if err := c.registry.Dispatch(ctx, &event); err != nil {
		logger.Error("failed to handle event", "error", err)
		return
	}

	logger.Debug("event processed successfully")
}

// Close releases consumer resources.
func (c *Consumer) Close() error {
	c.client.Close()
	c.logger.Info("event consumer closed")
	return nil
}
