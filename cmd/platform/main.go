package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cornjacket/platform-services/internal/services/eventhandler"
	"github.com/cornjacket/platform-services/internal/services/ingestion"
	"github.com/cornjacket/platform-services/internal/services/outbox"
	"github.com/cornjacket/platform-services/internal/shared/config"
	"github.com/cornjacket/platform-services/internal/shared/infra/postgres"
	"github.com/cornjacket/platform-services/internal/shared/infra/redpanda"
)

func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	slog.Info("starting platform services",
		"ingestion_port", cfg.PortIngestion,
		"query_port", cfg.PortQuery,
		"actions_port", cfg.PortActions,
	)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize PostgreSQL client for Ingestion service
	ingestionPG, err := postgres.NewClient(ctx, cfg.DatabaseURLIngestion, logger)
	if err != nil {
		slog.Error("failed to connect to PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer ingestionPG.Close()

	// Initialize repositories
	outboxRepo := postgres.NewOutboxRepo(ingestionPG.Pool(), logger)

	// Initialize ingestion service
	ingestionService := ingestion.NewService(outboxRepo, logger)
	ingestionHandler := ingestion.NewHandler(ingestionService, logger)

	// Set up HTTP server for ingestion
	ingestionMux := http.NewServeMux()
	ingestionHandler.RegisterRoutes(ingestionMux)

	ingestionServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.PortIngestion),
		Handler:      ingestionMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start ingestion server in goroutine
	go func() {
		slog.Info("starting ingestion server", "port", cfg.PortIngestion)
		if err := ingestionServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("ingestion server error", "error", err)
			cancel()
		}
	}()

	// Initialize Outbox Processor
	// Create dedicated LISTEN connection (not from pool)
	listenConn, err := pgx.Connect(ctx, cfg.DatabaseURLIngestion)
	if err != nil {
		slog.Error("failed to create LISTEN connection", "error", err)
		os.Exit(1)
	}
	defer listenConn.Close(context.Background())

	// Create event store repository
	eventStoreRepo := postgres.NewEventStoreRepo(ingestionPG.Pool(), logger)

	// Create Redpanda producer
	brokers := strings.Split(cfg.RedpandaBrokers, ",")
	redpandaProducer, err := redpanda.NewProducer(brokers, logger)
	if err != nil {
		slog.Error("failed to create Redpanda producer", "error", err)
		os.Exit(1)
	}
	defer redpandaProducer.Close()

	// Create outbox processor
	outboxProcessor := outbox.NewProcessor(
		postgres.NewOutboxReaderAdapter(ingestionPG.Pool(), logger),
		eventStoreRepo,
		redpandaProducer,
		listenConn,
		outbox.ProcessorConfig{
			WorkerCount:  cfg.OutboxWorkerCount,
			BatchSize:    cfg.OutboxBatchSize,
			MaxRetries:   cfg.OutboxMaxRetries,
			PollInterval: cfg.OutboxPollInterval,
		},
		logger,
	)

	// Start outbox processor in goroutine
	go func() {
		if err := outboxProcessor.Start(ctx); err != nil {
			slog.Error("outbox processor error", "error", err)
			cancel()
		}
	}()

	// Initialize Event Handler
	// Create PostgreSQL client for Event Handler service
	eventHandlerPG, err := postgres.NewClient(ctx, cfg.DatabaseURLEventHandler, logger)
	if err != nil {
		slog.Error("failed to connect to PostgreSQL for event handler", "error", err)
		os.Exit(1)
	}
	defer eventHandlerPG.Close()

	// Create projection repository
	projectionRepo := postgres.NewProjectionRepo(eventHandlerPG.Pool(), logger)

	// Create handler registry and register handlers
	handlerRegistry := eventhandler.NewHandlerRegistry(logger)
	handlerRegistry.Register("sensor.", eventhandler.NewSensorHandler(projectionRepo, logger))
	handlerRegistry.Register("user.", eventhandler.NewUserHandler(projectionRepo, logger))

	// Create event consumer
	topics := strings.Split(cfg.EventHandlerTopics, ",")
	eventConsumer, err := eventhandler.NewConsumer(
		handlerRegistry,
		eventhandler.ConsumerConfig{
			Brokers:     brokers,
			GroupID:     cfg.EventHandlerConsumerGroup,
			Topics:      topics,
			PollTimeout: cfg.EventHandlerPollTimeout,
		},
		logger,
	)
	if err != nil {
		slog.Error("failed to create event consumer", "error", err)
		os.Exit(1)
	}
	defer eventConsumer.Close()

	// Start event consumer in goroutine
	go func() {
		if err := eventConsumer.Start(ctx); err != nil {
			slog.Error("event consumer error", "error", err)
			cancel()
		}
	}()

	// TODO: Initialize and start Query service
	// TODO: Initialize and start Actions service

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		slog.Info("received shutdown signal", "signal", sig)
	case <-ctx.Done():
		slog.Info("context cancelled")
	}

	// Graceful shutdown
	slog.Info("shutting down servers...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := ingestionServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("ingestion server shutdown error", "error", err)
	}

	slog.Info("platform services stopped")
}
