package eventhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cornjacket/platform-services/internal/shared/domain/events"
)

// mockEventPublisher implements EventPublisher for testing.
type mockEventPublisher struct {
	PublishFn func(ctx context.Context, topic string, event *events.Envelope) error
}

func (m *mockEventPublisher) Publish(ctx context.Context, topic string, event *events.Envelope) error {
	return m.PublishFn(ctx, topic, event)
}

func TestSubmitEvent_Success(t *testing.T) {
	var capturedTopic string
	mock := &mockEventPublisher{
		PublishFn: func(ctx context.Context, topic string, event *events.Envelope) error {
			capturedTopic = topic
			return nil
		},
	}
	client := New(mock, slog.Default())

	envelope, _ := events.NewEnvelope(
		"sensor.reading", "device-001",
		json.RawMessage(`{"value": 72.5}`),
		events.Metadata{Source: "test"}, time.Now(),
	)

	err := client.SubmitEvent(context.Background(), envelope)
	require.NoError(t, err)
	assert.Equal(t, "sensor-events", capturedTopic)
}

func TestSubmitEvent_PublishError(t *testing.T) {
	mock := &mockEventPublisher{
		PublishFn: func(ctx context.Context, topic string, event *events.Envelope) error {
			return fmt.Errorf("broker unavailable")
		},
	}
	client := New(mock, slog.Default())

	envelope, _ := events.NewEnvelope(
		"sensor.reading", "device-001",
		json.RawMessage(`{"value": 72.5}`),
		events.Metadata{Source: "test"}, time.Now(),
	)

	err := client.SubmitEvent(context.Background(), envelope)
	assert.Error(t, err)
}

func TestTopicFromEventType(t *testing.T) {
	tests := []struct {
		eventType string
		want      string
	}{
		{"sensor.reading", "sensor-events"},
		{"sensor.alert", "sensor-events"},
		{"sensor.calibration", "sensor-events"},
		{"user.login", "user-actions"},
		{"user.logout", "user-actions"},
		{"user.signup", "user-actions"},
		{"system.startup", "system-events"},
		{"unknown.type", "system-events"},
		{"", "system-events"},
		{"no-prefix", "system-events"},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			assert.Equal(t, tt.want, topicFromEventType(tt.eventType))
		})
	}
}
