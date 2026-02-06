package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Default database URL for local development (all services share one DB)
const defaultDatabaseURL = "postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable"

// Config holds all configuration for the platform services.
type Config struct {
	// Server ports
	PortIngestion int
	PortQuery     int
	PortActions   int

	// Per-service database URLs (ADR-0010)
	DatabaseURLIngestion    string
	DatabaseURLEventHandler string
	DatabaseURLQuery        string
	DatabaseURLTSDB         string
	DatabaseURLActions      string

	// Redpanda
	RedpandaBrokers string

	// Outbox processor
	OutboxWorkerCount  int
	OutboxBatchSize    int
	OutboxMaxRetries   int
	OutboxPollInterval time.Duration

	// Event handler
	EventHandlerConsumerGroup string
	EventHandlerTopics        string
	EventHandlerPollTimeout   time.Duration

	// Feature flags
	EnableTSDB bool
}

// Load reads configuration from environment variables with sensible defaults.
// Environment variable naming convention: CJ_[SERVICE]_[VARIABLE_NAME]
// See design-spec.md section 12 for complete reference.
func Load() (*Config, error) {
	cfg := &Config{
		// Server ports
		PortIngestion: getEnvInt("CJ_INGESTION_PORT", 8080),
		PortQuery:     getEnvInt("CJ_QUERY_PORT", 8081),
		PortActions:   getEnvInt("CJ_ACTIONS_PORT", 8083), // Note: 8082 used by Redpanda Pandaproxy locally

		// Per-service database URLs
		// In dev, all default to the same database
		// In prod, each service gets its own database
		DatabaseURLIngestion:    getEnv("CJ_INGESTION_DATABASE_URL", defaultDatabaseURL),
		DatabaseURLEventHandler: getEnv("CJ_EVENTHANDLER_DATABASE_URL", defaultDatabaseURL),
		DatabaseURLQuery:        getEnv("CJ_QUERY_DATABASE_URL", defaultDatabaseURL),
		DatabaseURLTSDB:         getEnv("CJ_TSDB_DATABASE_URL", defaultDatabaseURL),
		DatabaseURLActions:      getEnv("CJ_ACTIONS_DATABASE_URL", defaultDatabaseURL),

		// Redpanda
		RedpandaBrokers: getEnv("CJ_REDPANDA_BROKERS", "localhost:9092"),

		// Outbox processor
		OutboxWorkerCount:  getEnvInt("CJ_OUTBOX_WORKER_COUNT", 4),
		OutboxBatchSize:    getEnvInt("CJ_OUTBOX_BATCH_SIZE", 100),
		OutboxMaxRetries:   getEnvInt("CJ_OUTBOX_MAX_RETRIES", 5),
		OutboxPollInterval: getEnvDuration("CJ_OUTBOX_POLL_INTERVAL", 5*time.Second),

		// Event handler
		EventHandlerConsumerGroup: getEnv("CJ_EVENTHANDLER_CONSUMER_GROUP", "event-handler"),
		EventHandlerTopics:        getEnv("CJ_EVENTHANDLER_TOPICS", "sensor-events,user-actions,system-events"),
		EventHandlerPollTimeout:   getEnvDuration("CJ_EVENTHANDLER_POLL_TIMEOUT", 1*time.Second),

		// Feature flags
		EnableTSDB: getEnvBool("CJ_FEATURE_TSDB", false),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.DatabaseURLIngestion == "" {
		return fmt.Errorf("CJ_INGESTION_DATABASE_URL is required")
	}
	if c.RedpandaBrokers == "" {
		return fmt.Errorf("CJ_REDPANDA_BROKERS is required")
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
