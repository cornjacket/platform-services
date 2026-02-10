package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cornjacket/platform-services/internal/shared/domain/clock"
)

func TestNewEnvelope(t *testing.T) {
	fixedIngestTime := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	clock.Set(clock.FixedClock{Time: fixedIngestTime})
	t.Cleanup(clock.Reset)

	eventTime := time.Date(2026, 2, 7, 10, 0, 0, 0, time.UTC)
	payload := map[string]any{"value": 72.5, "unit": "fahrenheit"}
	metadata := Metadata{TraceID: "trace-123", Source: "test", SchemaVersion: 1}

	envelope, err := NewEnvelope("sensor.reading", "device-001", payload, metadata, eventTime)
	require.NoError(t, err)

	assert.False(t, envelope.EventID.IsNil(), "EventID should not be nil")
	assert.Equal(t, "sensor.reading", envelope.EventType)
	assert.Equal(t, "device-001", envelope.AggregateID)
	assert.Equal(t, eventTime, envelope.EventTime)
	assert.Equal(t, fixedIngestTime, envelope.IngestedAt)
	assert.Equal(t, "trace-123", envelope.Metadata.TraceID)
}

func TestNewEnvelope_PayloadMarshaling(t *testing.T) {
	clock.Set(clock.FixedClock{Time: time.Now()})
	t.Cleanup(clock.Reset)

	payload := map[string]any{
		"nested": map[string]any{"key": "value"},
		"array":  []int{1, 2, 3},
	}

	envelope, err := NewEnvelope("test.event", "agg-1", payload, Metadata{}, time.Now())
	require.NoError(t, err)

	var parsed map[string]any
	assert.NoError(t, json.Unmarshal(envelope.Payload, &parsed))
}

func TestEnvelope_ParsePayload(t *testing.T) {
	clock.Set(clock.FixedClock{Time: time.Now()})
	t.Cleanup(clock.Reset)

	type SensorReading struct {
		Value float64 `json:"value"`
		Unit  string  `json:"unit"`
	}

	original := SensorReading{Value: 72.5, Unit: "fahrenheit"}
	envelope, err := NewEnvelope("sensor.reading", "device-001", original, Metadata{}, time.Now())
	require.NoError(t, err)

	var parsed SensorReading
	require.NoError(t, envelope.ParsePayload(&parsed))

	assert.Equal(t, original.Value, parsed.Value)
	assert.Equal(t, original.Unit, parsed.Unit)
}

func TestNewEnvelope_InvalidPayload(t *testing.T) {
	clock.Set(clock.FixedClock{Time: time.Now()})
	t.Cleanup(clock.Reset)

	_, err := NewEnvelope("test.event", "agg-1", make(chan int), Metadata{}, time.Now())
	assert.Error(t, err)
}

func TestNewEnvelope_DualTimestamps(t *testing.T) {
	eventTime := time.Date(2026, 2, 7, 10, 0, 0, 0, time.UTC)
	ingestTime := time.Date(2026, 2, 7, 10, 15, 0, 0, time.UTC)

	clock.Set(clock.FixedClock{Time: ingestTime})
	t.Cleanup(clock.Reset)

	envelope, err := NewEnvelope("sensor.reading", "device-001", map[string]any{}, Metadata{}, eventTime)
	require.NoError(t, err)

	assert.Equal(t, eventTime, envelope.EventTime)
	assert.Equal(t, ingestTime, envelope.IngestedAt)
	assert.Equal(t, 15*time.Minute, envelope.IngestedAt.Sub(envelope.EventTime))
}
