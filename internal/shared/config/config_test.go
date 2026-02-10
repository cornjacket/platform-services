package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config",
			cfg:     &Config{DatabaseURLIngestion: "postgres://localhost/db", RedpandaBrokers: "localhost:9092"},
			wantErr: false,
		},
		{
			name:    "missing database URL",
			cfg:     &Config{DatabaseURLIngestion: "", RedpandaBrokers: "localhost:9092"},
			wantErr: true,
			errMsg:  "CJ_INGESTION_DATABASE_URL is required",
		},
		{
			name:    "missing Redpanda brokers",
			cfg:     &Config{DatabaseURLIngestion: "postgres://localhost/db", RedpandaBrokers: ""},
			wantErr: true,
			errMsg:  "CJ_REDPANDA_BROKERS is required",
		},
		{
			name:    "both missing - first error wins",
			cfg:     &Config{DatabaseURLIngestion: "", RedpandaBrokers: ""},
			wantErr: true,
			errMsg:  "CJ_INGESTION_DATABASE_URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, "json", cfg.LogFormat)
	assert.Equal(t, 8080, cfg.PortIngestion)
	assert.Equal(t, 8081, cfg.PortQuery)
	assert.Equal(t, 4, cfg.OutboxWorkerCount)
	assert.Equal(t, 100, cfg.OutboxBatchSize)
	assert.Equal(t, 5, cfg.OutboxMaxRetries)
	assert.Equal(t, false, cfg.EnableTSDB)
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("CJ_LOG_LEVEL", "debug")
	t.Setenv("CJ_INGESTION_PORT", "9090")
	t.Setenv("CJ_OUTBOX_WORKER_COUNT", "8")
	t.Setenv("CJ_FEATURE_TSDB", "true")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, 9090, cfg.PortIngestion)
	assert.Equal(t, 8, cfg.OutboxWorkerCount)
	assert.Equal(t, true, cfg.EnableTSDB)
}

func TestLoad_CustomDatabaseURL(t *testing.T) {
	customURL := "postgres://custom:5432/testdb"
	os.Setenv("CJ_INGESTION_DATABASE_URL", customURL)
	defer os.Unsetenv("CJ_INGESTION_DATABASE_URL")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, customURL, cfg.DatabaseURLIngestion)
}
