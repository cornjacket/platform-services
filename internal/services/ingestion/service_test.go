package ingestion

import (
	"encoding/json"
	"testing"
)

func TestValidate(t *testing.T) {
	service := &Service{} // validate doesn't use any dependencies

	tests := []struct {
		name    string
		req     *IngestRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid request",
			req: &IngestRequest{
				EventType:   "sensor.reading",
				AggregateID: "device-001",
				Payload:     json.RawMessage(`{"value": 72.5}`),
			},
			wantErr: false,
		},
		{
			name: "missing event_type",
			req: &IngestRequest{
				AggregateID: "device-001",
				Payload:     json.RawMessage(`{"value": 72.5}`),
			},
			wantErr: true,
			errMsg:  "event_type is required",
		},
		{
			name: "missing aggregate_id",
			req: &IngestRequest{
				EventType: "sensor.reading",
				Payload:   json.RawMessage(`{"value": 72.5}`),
			},
			wantErr: true,
			errMsg:  "aggregate_id is required",
		},
		{
			name: "missing payload",
			req: &IngestRequest{
				EventType:   "sensor.reading",
				AggregateID: "device-001",
			},
			wantErr: true,
			errMsg:  "payload is required",
		},
		{
			name: "empty payload",
			req: &IngestRequest{
				EventType:   "sensor.reading",
				AggregateID: "device-001",
				Payload:     json.RawMessage(``),
			},
			wantErr: true,
			errMsg:  "payload is required",
		},
		{
			name: "invalid JSON payload",
			req: &IngestRequest{
				EventType:   "sensor.reading",
				AggregateID: "device-001",
				Payload:     json.RawMessage(`{invalid json}`),
			},
			wantErr: true,
			errMsg:  "payload must be valid JSON",
		},
		{
			name: "null payload is valid JSON",
			req: &IngestRequest{
				EventType:   "sensor.reading",
				AggregateID: "device-001",
				Payload:     json.RawMessage(`null`),
			},
			wantErr: false,
		},
		{
			name: "array payload is valid",
			req: &IngestRequest{
				EventType:   "sensor.reading",
				AggregateID: "device-001",
				Payload:     json.RawMessage(`[1, 2, 3]`),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validate(tt.req)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validate() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("validate() error = %q, want error containing %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validate() unexpected error: %v", err)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
