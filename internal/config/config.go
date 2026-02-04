package config

import (
	"fmt"
	"os"
	"strconv"
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

	// Feature flags
	EnableTSDB bool
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		// Server ports
		PortIngestion: getEnvInt("PORT_INGESTION", 8080),
		PortQuery:     getEnvInt("PORT_QUERY", 8081),
		PortActions:   getEnvInt("PORT_ACTIONS", 8083), // Note: 8082 used by Redpanda Pandaproxy locally

		// Per-service database URLs
		// In dev, all default to the same database
		// In prod, each service gets its own database
		DatabaseURLIngestion:    getEnv("INGESTION_DATABASE_URL", defaultDatabaseURL),
		DatabaseURLEventHandler: getEnv("EVENTHANDLER_DATABASE_URL", defaultDatabaseURL),
		DatabaseURLQuery:        getEnv("QUERY_DATABASE_URL", defaultDatabaseURL),
		DatabaseURLTSDB:         getEnv("TSDB_DATABASE_URL", defaultDatabaseURL),
		DatabaseURLActions:      getEnv("ACTIONS_DATABASE_URL", defaultDatabaseURL),

		// Redpanda
		RedpandaBrokers: getEnv("REDPANDA_BROKERS", "localhost:9092"),

		// Feature flags
		EnableTSDB: getEnvBool("ENABLE_TSDB", false),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.DatabaseURLIngestion == "" {
		return fmt.Errorf("INGESTION_DATABASE_URL is required")
	}
	if c.RedpandaBrokers == "" {
		return fmt.Errorf("REDPANDA_BROKERS is required")
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
