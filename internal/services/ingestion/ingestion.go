package ingestion

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cornjacket/platform-services/internal/services/ingestion/worker"
	"github.com/cornjacket/platform-services/internal/shared/infra/postgres"
)

// Config holds configuration for the ingestion service.
type Config struct {
	Port         int
	WorkerCount  int
	BatchSize    int
	MaxRetries   int
	PollInterval time.Duration
	DatabaseURL  string // needed for dedicated LISTEN connection (separate from pool)
}

// RunningService represents a started ingestion service.
type RunningService struct {
	// Shutdown stops the HTTP server and worker gracefully.
	Shutdown func(ctx context.Context) error
}

// Start starts the ingestion HTTP server and outbox worker.
// It creates all internal wiring (repos, handlers, routes) from the provided pool.
// The submitter is the service's output — where processed events are sent downstream.
func Start(ctx context.Context, cfg Config, pool *pgxpool.Pool, submitter worker.EventSubmitter, logger *slog.Logger) (*RunningService, error) {
	logger = logger.With("service", "ingestion")

	// Create repositories from pool
	outboxRepo := postgres.NewOutboxRepo(pool, logger)
	eventStoreRepo := postgres.NewEventStoreRepo(pool, logger)
	outboxReader := postgres.NewOutboxReaderAdapter(pool, logger)

	// Create dedicated LISTEN connection (not from pool — holds connection open indefinitely)
	listenConn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create LISTEN connection: %w", err)
	}

	// Wire service → handler → routes → HTTP server
	svc := NewService(outboxRepo, logger)
	handler := NewHandler(svc, logger)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Wire outbox worker
	proc := worker.NewProcessor(
		outboxReader,
		eventStoreRepo,
		submitter,
		listenConn,
		worker.ProcessorConfig{
			WorkerCount:  cfg.WorkerCount,
			BatchSize:    cfg.BatchSize,
			MaxRetries:   cfg.MaxRetries,
			PollInterval: cfg.PollInterval,
		},
		logger,
	)

	// Start HTTP server
	go func() {
		logger.Info("starting ingestion server", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("ingestion server error", "error", err)
		}
	}()

	// Start outbox worker
	go func() {
		if err := proc.Start(ctx); err != nil {
			logger.Error("ingestion worker error", "error", err)
		}
	}()

	return &RunningService{
		Shutdown: func(shutdownCtx context.Context) error {
			logger.Info("shutting down ingestion service")
			listenConn.Close(shutdownCtx)
			return server.Shutdown(shutdownCtx)
		},
	}, nil
}
