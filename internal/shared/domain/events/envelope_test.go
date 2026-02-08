package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cornjacket/platform-services/internal/shared/domain/clock"
)

func TestNewEnvelope(t *testing.T) {
	// Set fixed clock for deterministic testing
	fixedIngestTime := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	clock.Set(clock.FixedClock{Time: fixedIngestTime})
	t.Cleanup(clock.Reset)

	eventTime := time.Date(2026, 2, 7, 10, 0, 0, 0, time.UTC) // 2 hours before ingestion
	payload := map[string]any{"value": 72.5, "unit": "fahrenheit"}
	metadata := Metadata{
		TraceID:       "trace-123",
		Source:        "test",
		SchemaVersion: 1,
	}

	envelope, err := NewEnvelope("sensor.reading", "device-001", payload, metadata, eventTime)
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}

	// Check EventID is a valid UUID v7 (non-zero)
	if envelope.EventID.IsNil() {
		t.Error("EventID should not be nil")
	}

	// Check EventType
	if envelope.EventType != "sensor.reading" {
		t.Errorf("EventType = %v, want sensor.reading", envelope.EventType)
	}

	// Check AggregateID
	if envelope.AggregateID != "device-001" {
		t.Errorf("AggregateID = %v, want device-001", envelope.AggregateID)
	}

	// Check EventTime (from caller)
	if !envelope.EventTime.Equal(eventTime) {
		t.Errorf("EventTime = %v, want %v", envelope.EventTime, eventTime)
	}

	// Check IngestedAt (from clock)
	if !envelope.IngestedAt.Equal(fixedIngestTime) {
		t.Errorf("IngestedAt = %v, want %v", envelope.IngestedAt, fixedIngestTime)
	}

	// Check Metadata
	if envelope.Metadata.TraceID != "trace-123" {
		t.Errorf("Metadata.TraceID = %v, want trace-123", envelope.Metadata.TraceID)
	}
}

func TestNewEnvelope_PayloadMarshaling(t *testing.T) {
	clock.Set(clock.FixedClock{Time: time.Now()})
	t.Cleanup(clock.Reset)

	payload := map[string]any{
		"nested": map[string]any{
			"key": "value",
		},
		"array": []int{1, 2, 3},
	}

	envelope, err := NewEnvelope("test.event", "agg-1", payload, Metadata{}, time.Now())
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}

	// Verify payload is valid JSON
	var parsed map[string]any
	if err := json.Unmarshal(envelope.Payload, &parsed); err != nil {
		t.Errorf("Payload is not valid JSON: %v", err)
	}
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
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}

	var parsed SensorReading
	if err := envelope.ParsePayload(&parsed); err != nil {
		t.Fatalf("ParsePayload() error = %v", err)
	}

	if parsed.Value != original.Value {
		t.Errorf("ParsePayload().Value = %v, want %v", parsed.Value, original.Value)
	}
	if parsed.Unit != original.Unit {
		t.Errorf("ParsePayload().Unit = %v, want %v", parsed.Unit, original.Unit)
	}
}

func TestNewEnvelope_InvalidPayload(t *testing.T) {
	clock.Set(clock.FixedClock{Time: time.Now()})
	t.Cleanup(clock.Reset)

	// channels cannot be marshaled to JSON
	invalidPayload := make(chan int)

	_, err := NewEnvelope("test.event", "agg-1", invalidPayload, Metadata{}, time.Now())
	if err == nil {
		t.Error("NewEnvelope() with invalid payload should return error")
	}
}

func TestNewEnvelope_DualTimestamps(t *testing.T) {
	// Simulate IoT scenario: event happened 15 minutes ago, just now ingested
	eventTime := time.Date(2026, 2, 7, 10, 0, 0, 0, time.UTC)
	ingestTime := time.Date(2026, 2, 7, 10, 15, 0, 0, time.UTC)

	clock.Set(clock.FixedClock{Time: ingestTime})
	t.Cleanup(clock.Reset)

	envelope, err := NewEnvelope("sensor.reading", "device-001", map[string]any{}, Metadata{}, eventTime)
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}

	// EventTime should be the original event time (15 minutes ago)
	if !envelope.EventTime.Equal(eventTime) {
		t.Errorf("EventTime = %v, want %v", envelope.EventTime, eventTime)
	}

	// IngestedAt should be the ingestion time (now)
	if !envelope.IngestedAt.Equal(ingestTime) {
		t.Errorf("IngestedAt = %v, want %v", envelope.IngestedAt, ingestTime)
	}

	// Verify the 15 minute difference
	diff := envelope.IngestedAt.Sub(envelope.EventTime)
	if diff != 15*time.Minute {
		t.Errorf("IngestedAt - EventTime = %v, want 15 minutes", diff)
	}
}
