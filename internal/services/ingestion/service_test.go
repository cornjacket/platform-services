package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cornjacket/platform-services/internal/shared/domain/clock"
	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

func TestValidate(t *testing.T) {
	service := &Service{}

	tests := []struct {
		name    string
		req     *IngestRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid request",
			req:     &IngestRequest{EventType: "sensor.reading", AggregateID: "device-001", Payload: json.RawMessage(`{"value": 72.5}`)},
			wantErr: false,
		},
		{
			name:    "missing event_type",
			req:     &IngestRequest{AggregateID: "device-001", Payload: json.RawMessage(`{"value": 72.5}`)},
			wantErr: true, errMsg: "event_type is required",
		},
		{
			name:    "missing aggregate_id",
			req:     &IngestRequest{EventType: "sensor.reading", Payload: json.RawMessage(`{"value": 72.5}`)},
			wantErr: true, errMsg: "aggregate_id is required",
		},
		{
			name:    "missing payload",
			req:     &IngestRequest{EventType: "sensor.reading", AggregateID: "device-001"},
			wantErr: true, errMsg: "payload is required",
		},
		{
			name:    "empty payload",
			req:     &IngestRequest{EventType: "sensor.reading", AggregateID: "device-001", Payload: json.RawMessage(``)},
			wantErr: true, errMsg: "payload is required",
		},
		{
			name:    "invalid JSON payload",
			req:     &IngestRequest{EventType: "sensor.reading", AggregateID: "device-001", Payload: json.RawMessage(`{invalid json}`)},
			wantErr: true, errMsg: "payload must be valid JSON",
		},
		{
			name:    "null payload is valid JSON",
			req:     &IngestRequest{EventType: "sensor.reading", AggregateID: "device-001", Payload: json.RawMessage(`null`)},
			wantErr: false,
		},
		{
			name:    "array payload is valid",
			req:     &IngestRequest{EventType: "sensor.reading", AggregateID: "device-001", Payload: json.RawMessage(`[1, 2, 3]`)},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.validate(tt.req)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIngest_Success(t *testing.T) {
	fixedTime := time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)
	clock.Set(clock.FixedClock{Time: fixedTime})
	t.Cleanup(clock.Reset)

	var captured *events.Envelope
	mock := &mockOutboxRepository{
		InsertFn: func(ctx context.Context, event *events.Envelope) error {
			captured = event
			return nil
		},
	}
	service := NewService(mock, slog.Default())

	req := &IngestRequest{
		EventType:   "sensor.reading",
		AggregateID: "device-001",
		Payload:     json.RawMessage(`{"value": 72.5}`),
		TraceID:     "trace-abc",
	}

	resp, err := service.Ingest(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)
	assert.NotEmpty(t, resp.EventID)

	require.NotNil(t, captured)
	assert.Equal(t, "sensor.reading", captured.EventType)
	assert.Equal(t, "device-001", captured.AggregateID)
	assert.Equal(t, "trace-abc", captured.Metadata.TraceID)
	assert.Equal(t, fixedTime, captured.IngestedAt)
}

func TestIngest_WithEventTime(t *testing.T) {
	fixedTime := time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC)
	clock.Set(clock.FixedClock{Time: fixedTime})
	t.Cleanup(clock.Reset)

	var captured *events.Envelope
	mock := &mockOutboxRepository{
		InsertFn: func(ctx context.Context, event *events.Envelope) error {
			captured = event
			return nil
		},
	}
	service := NewService(mock, slog.Default())

	eventTime := time.Date(2026, 2, 9, 11, 45, 0, 0, time.UTC)
	req := &IngestRequest{
		EventType:   "sensor.reading",
		AggregateID: "device-001",
		Payload:     json.RawMessage(`{"value": 72.5}`),
		EventTime:   &eventTime,
	}

	_, err := service.Ingest(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, eventTime, captured.EventTime)
	assert.Equal(t, fixedTime, captured.IngestedAt, "IngestedAt should use clock, not event_time")
}

func TestIngest_ValidationFailure(t *testing.T) {
	mock := &mockOutboxRepository{
		InsertFn: func(ctx context.Context, event *events.Envelope) error {
			t.Fatal("Insert should not be called for invalid request")
			return nil
		},
	}
	service := NewService(mock, slog.Default())

	req := &IngestRequest{AggregateID: "device-001", Payload: json.RawMessage(`{"value": 72.5}`)}

	_, err := service.Ingest(context.Background(), req)
	assert.Error(t, err)
}

func TestIngest_OutboxError(t *testing.T) {
	clock.Set(clock.FixedClock{Time: time.Now()})
	t.Cleanup(clock.Reset)

	mock := &mockOutboxRepository{
		InsertFn: func(ctx context.Context, event *events.Envelope) error {
			return fmt.Errorf("connection refused")
		},
	}
	service := NewService(mock, slog.Default())

	req := &IngestRequest{
		EventType:   "sensor.reading",
		AggregateID: "device-001",
		Payload:     json.RawMessage(`{"value": 72.5}`),
	}

	_, err := service.Ingest(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outbox")
}
