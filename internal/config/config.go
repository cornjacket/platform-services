package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all configuration for the platform services.
type Config struct {
	// Server ports
	PortIngestion int
	PortQuery     int
	PortActions   int

	// Database
	DatabaseURL string

	// Redpanda
	RedpandaBrokers string

	// Feature flags
	EnableTSDB bool
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		PortIngestion:   getEnvInt("PORT_INGESTION", 8080),
		PortQuery:       getEnvInt("PORT_QUERY", 8081),
		PortActions:     getEnvInt("PORT_ACTIONS", 8083), // Note: 8082 used by Redpanda Pandaproxy locally
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable"),
		RedpandaBrokers: getEnv("REDPANDA_BROKERS", "localhost:9092"),
		EnableTSDB:      getEnvBool("ENABLE_TSDB", false),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
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
