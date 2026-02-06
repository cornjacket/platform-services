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
		Name:        "full-flow",
		Description: "Complete flow: ingest event, update with newer event, verify state",
		Run:         runFullFlowTest,
	})
}

func runFullFlowTest(ctx context.Context, cfg *runner.Config) error {
	c := &client.Config{
		IngestionURL: cfg.IngestionURL,
		QueryURL:     cfg.QueryURL,
	}

	// Generate unique aggregate ID for test isolation
	aggregateID := client.UniqueID("e2e-sensor")

	// 1. Ingest initial event
	req1 := &client.IngestRequest{
		EventType:   "sensor.reading",
		AggregateID: aggregateID,
		Payload: map[string]interface{}{
			"value": 70.0,
			"unit":  "fahrenheit",
		},
	}

	resp1, err := client.IngestEvent(ctx, c, req1)
	if err != nil {
		return fmt.Errorf("failed to ingest first event: %w", err)
	}

	// 2. Wait for initial projection
	projection1, err := client.WaitForProjection(ctx, c, "sensor_state", aggregateID, 5*time.Second)
	if err != nil {
		return fmt.Errorf("initial projection not created: %w", err)
	}

	// Verify initial state
	var state1 map[string]interface{}
	if err := json.Unmarshal(projection1.State, &state1); err != nil {
		return fmt.Errorf("failed to unmarshal initial state: %w", err)
	}

	if state1["value"].(float64) != 70.0 {
		return fmt.Errorf("expected initial value 70.0, got %v", state1["value"])
	}

	initialEventID := projection1.LastEventID

	// 3. Ingest update event
	req2 := &client.IngestRequest{
		EventType:   "sensor.reading",
		AggregateID: aggregateID,
		Payload: map[string]interface{}{
			"value": 75.5,
			"unit":  "fahrenheit",
		},
	}

	resp2, err := client.IngestEvent(ctx, c, req2)
	if err != nil {
		return fmt.Errorf("failed to ingest second event: %w", err)
	}

	// Verify we got different event IDs
	if resp1.EventID == resp2.EventID {
		return fmt.Errorf("expected different event IDs for two ingests")
	}

	// 4. Wait for projection to update
	// Poll until the projection's last_event_id changes
	deadline := time.Now().Add(5 * time.Second)
	var projection2 *client.Projection
	for time.Now().Before(deadline) {
		projection2, err = client.GetProjection(ctx, c, "sensor_state", aggregateID)
		if err != nil {
			return fmt.Errorf("failed to get updated projection: %w", err)
		}

		if projection2.LastEventID != initialEventID {
			break // Projection was updated
		}

		time.Sleep(100 * time.Millisecond)
	}

	if projection2.LastEventID == initialEventID {
		return fmt.Errorf("projection was not updated after second event")
	}

	// 5. Verify updated state
	var state2 map[string]interface{}
	if err := json.Unmarshal(projection2.State, &state2); err != nil {
		return fmt.Errorf("failed to unmarshal updated state: %w", err)
	}

	if state2["value"].(float64) != 75.5 {
		return fmt.Errorf("expected updated value 75.5, got %v", state2["value"])
	}

	return nil
}
