package config

import "testing"

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			cfg: &Config{
				DatabaseURLIngestion: "postgres://localhost/db",
				RedpandaBrokers:      "localhost:9092",
			},
			wantErr: false,
		},
		{
			name: "missing database URL",
			cfg: &Config{
				DatabaseURLIngestion: "",
				RedpandaBrokers:      "localhost:9092",
			},
			wantErr: true,
			errMsg:  "CJ_INGESTION_DATABASE_URL is required",
		},
		{
			name: "missing Redpanda brokers",
			cfg: &Config{
				DatabaseURLIngestion: "postgres://localhost/db",
				RedpandaBrokers:      "",
			},
			wantErr: true,
			errMsg:  "CJ_REDPANDA_BROKERS is required",
		},
		{
			name: "both missing - first error wins",
			cfg: &Config{
				DatabaseURLIngestion: "",
				RedpandaBrokers:      "",
			},
			wantErr: true,
			errMsg:  "CJ_INGESTION_DATABASE_URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("validate() expected error, got nil")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("validate() error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validate() unexpected error: %v", err)
				}
			}
		})
	}
}
