package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	ehclient "github.com/cornjacket/platform-services/internal/client/eventhandler"
	"github.com/cornjacket/platform-services/internal/services/eventhandler"
	"github.com/cornjacket/platform-services/internal/services/ingestion"
	"github.com/cornjacket/platform-services/internal/services/query"
	"github.com/cornjacket/platform-services/internal/shared/config"
	"github.com/cornjacket/platform-services/internal/shared/infra/postgres"
	"github.com/cornjacket/platform-services/internal/shared/infra/redpanda"
	"github.com/cornjacket/platform-services/internal/shared/projections"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := newLogger(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(logger)

	slog.Info("starting platform services",
		"ingestion_port", cfg.PortIngestion,
		"query_port", cfg.PortQuery,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create DB pools (one per service, per ADR-0010)
	ingestionPG, err := postgres.NewClient(ctx, cfg.DatabaseURLIngestion, logger)
	if err != nil {
		slog.Error("failed to connect to PostgreSQL (ingestion)", "error", err)
		os.Exit(1)
	}
	defer ingestionPG.Close()

	eventHandlerPG, err := postgres.NewClient(ctx, cfg.DatabaseURLEventHandler, logger)
	if err != nil {
		slog.Error("failed to connect to PostgreSQL (event handler)", "error", err)
		os.Exit(1)
	}
	defer eventHandlerPG.Close()

	queryPG, err := postgres.NewClient(ctx, cfg.DatabaseURLQuery, logger)
	if err != nil {
		slog.Error("failed to connect to PostgreSQL (query)", "error", err)
		os.Exit(1)
	}
	defer queryPG.Close()

	// Create shared external resources
	brokers := strings.Split(cfg.RedpandaBrokers, ",")
	redpandaProducer, err := redpanda.NewProducer(brokers, logger)
	if err != nil {
		slog.Error("failed to create Redpanda producer", "error", err)
		os.Exit(1)
	}
	defer redpandaProducer.Close()

	eventSubmitter := ehclient.New(redpandaProducer, logger)
	projectionsStore := projections.NewPostgresStore(eventHandlerPG.Pool(), logger)

	// Start services
	ingestionSvc, err := ingestion.Start(ctx, ingestion.Config{
		Port:         cfg.PortIngestion,
		WorkerCount:  cfg.OutboxWorkerCount,
		BatchSize:    cfg.OutboxBatchSize,
		MaxRetries:   cfg.OutboxMaxRetries,
		PollInterval: cfg.OutboxPollInterval,
		DatabaseURL:  cfg.DatabaseURLIngestion,
	}, ingestionPG.Pool(), eventSubmitter, logger)
	if err != nil {
		slog.Error("failed to start ingestion service", "error", err)
		os.Exit(1)
	}

	ehTopics := strings.Split(cfg.EventHandlerTopics, ",")
	eventHandlerSvc, err := eventhandler.Start(ctx, eventhandler.Config{
		Brokers:       brokers,
		ConsumerGroup: cfg.EventHandlerConsumerGroup,
		Topics:        ehTopics,
		PollTimeout:   cfg.EventHandlerPollTimeout,
	}, projectionsStore, logger)
	if err != nil {
		slog.Error("failed to start event handler service", "error", err)
		os.Exit(1)
	}

	querySvc, err := query.Start(ctx, query.Config{
		Port: cfg.PortQuery,
	}, queryPG.Pool(), logger)
	if err != nil {
		slog.Error("failed to start query service", "error", err)
		os.Exit(1)
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		slog.Info("received shutdown signal", "signal", sig)
	case <-ctx.Done():
		slog.Info("context cancelled")
	}

	// Graceful shutdown (reverse order)
	slog.Info("shutting down services...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := querySvc.Shutdown(shutdownCtx); err != nil {
		slog.Error("query service shutdown error", "error", err)
	}
	if err := eventHandlerSvc.Shutdown(shutdownCtx); err != nil {
		slog.Error("event handler service shutdown error", "error", err)
	}
	if err := ingestionSvc.Shutdown(shutdownCtx); err != nil {
		slog.Error("ingestion service shutdown error", "error", err)
	}

	slog.Info("platform services stopped")
}

// newLogger creates a structured logger based on configuration.
func newLogger(level, format string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: logLevel}

	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
