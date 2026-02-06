package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cornjacket/platform-services/e2e/client"
	"github.com/cornjacket/platform-services/e2e/runner"
)

func init() {
	runner.Register(&runner.Test{
		Name:        "ingest-event",
		Description: "Ingest an event and verify it creates a projection",
		Run:         runIngestEventTest,
	})
}

func runIngestEventTest(ctx context.Context, cfg *runner.Config) error {
	c := &client.Config{
		IngestionURL: cfg.IngestionURL,
		QueryURL:     cfg.QueryURL,
	}

	// Generate unique aggregate ID for test isolation
	aggregateID := client.UniqueID("e2e-device")

	// 1. Ingest a sensor.reading event
	req := &client.IngestRequest{
		EventType:   "sensor.reading",
		AggregateID: aggregateID,
		Payload: map[string]interface{}{
			"value": 72.5,
			"unit":  "fahrenheit",
		},
	}

	resp, err := client.IngestEvent(ctx, c, req)
	if err != nil {
		return fmt.Errorf("failed to ingest event: %w", err)
	}

	if resp.Status != "accepted" {
		return fmt.Errorf("expected status 'accepted', got '%s'", resp.Status)
	}

	if resp.EventID == "" {
		return fmt.Errorf("expected non-empty event_id")
	}

	// 2. Wait for projection to be created
	projection, err := client.WaitForProjection(ctx, c, "sensor_state", aggregateID, 5*time.Second)
	if err != nil {
		return fmt.Errorf("projection not created: %w", err)
	}

	// 3. Verify projection state
	var state map[string]interface{}
	if err := json.Unmarshal(projection.State, &state); err != nil {
		return fmt.Errorf("failed to unmarshal projection state: %w", err)
	}

	value, ok := state["value"].(float64)
	if !ok || value != 72.5 {
		return fmt.Errorf("expected value 72.5, got %v", state["value"])
	}

	unit, ok := state["unit"].(string)
	if !ok || unit != "fahrenheit" {
		return fmt.Errorf("expected unit 'fahrenheit', got %v", state["unit"])
	}

	return nil
}
