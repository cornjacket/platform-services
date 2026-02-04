package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Client manages the PostgreSQL connection pool.
type Client struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewClient creates a new PostgreSQL client with a connection pool.
func NewClient(ctx context.Context, databaseURL string, logger *slog.Logger) (*Client, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Configure pool settings
	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute
	config.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("connected to PostgreSQL",
		"max_conns", config.MaxConns,
		"min_conns", config.MinConns,
	)

	return &Client{
		pool:   pool,
		logger: logger.With("component", "postgres"),
	}, nil
}

// Pool returns the underlying connection pool.
func (c *Client) Pool() *pgxpool.Pool {
	return c.pool
}

// Close closes the connection pool.
func (c *Client) Close() {
	c.pool.Close()
	c.logger.Info("PostgreSQL connection pool closed")
}

// Health checks if the database is reachable.
func (c *Client) Health(ctx context.Context) error {
	return c.pool.Ping(ctx)
}
