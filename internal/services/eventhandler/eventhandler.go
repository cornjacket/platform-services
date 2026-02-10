package eventhandler

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Config holds configuration for the event handler service.
type Config struct {
	Brokers       []string
	ConsumerGroup string
	Topics        []string
	PollTimeout   time.Duration
}

// RunningService represents a started event handler service.
type RunningService struct {
	// Shutdown stops the consumer gracefully.
	Shutdown func(ctx context.Context) error
}

// Start starts the event handler consumer.
// The writer is the service's output — where projections are written for downstream consumers.
func Start(ctx context.Context, cfg Config, writer ProjectionWriter, logger *slog.Logger) (*RunningService, error) {
	logger = logger.With("service", "eventhandler")

	// Wire handler registry with event-type handlers
	registry := NewHandlerRegistry(logger)
	registry.Register("sensor.", NewSensorHandler(writer, logger))
	registry.Register("user.", NewUserHandler(writer, logger))

	// Create consumer
	consumer, err := NewConsumer(
		registry,
		ConsumerConfig{
			Brokers:     cfg.Brokers,
			GroupID:     cfg.ConsumerGroup,
			Topics:      cfg.Topics,
			PollTimeout: cfg.PollTimeout,
		},
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create event consumer: %w", err)
	}

	// Start consumer
	go func() {
		if err := consumer.Start(ctx); err != nil {
			logger.Error("event consumer error", "error", err)
		}
	}()

	return &RunningService{
		Shutdown: func(shutdownCtx context.Context) error {
			logger.Info("shutting down event handler service")
			return consumer.Close()
		},
	}, nil
}
