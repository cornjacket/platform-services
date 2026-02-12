package query

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cornjacket/platform-services/internal/shared/projections"
)

// Config holds configuration for the query service.
type Config struct {
	Port int
}

// RunningService represents a started query service.
type RunningService struct {
	// Shutdown stops the HTTP server gracefully.
	Shutdown func(ctx context.Context) error
}

// Start starts the query HTTP server.
// It creates the projections store from the provided pool and wires the service internally.
func Start(ctx context.Context, cfg Config, pool *pgxpool.Pool, logger *slog.Logger, errorCh chan<- error) (*RunningService, error) {
	logger = logger.With("service", "query")

	// Create projections store from pool
	store := projections.NewPostgresStore(pool, logger)

	// Wire service → handler → routes → HTTP server
	svc := NewService(store, logger)
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

	// Start HTTP server
	go func() {
		logger.Info("starting query server", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("query server error", "error", err)
			errorCh <- fmt.Errorf("query server failed: %w", err)
		}
	}()

	return &RunningService{
		Shutdown: func(shutdownCtx context.Context) error {
			logger.Info("shutting down query service")
			return server.Shutdown(shutdownCtx)
		},
	}, nil
}
