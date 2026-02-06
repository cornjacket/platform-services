package tests

import (
	"context"
	"fmt"
	"time"

	"github.com/cornjacket/platform-services/e2e/client"
	"github.com/cornjacket/platform-services/e2e/runner"
)

func init() {
	runner.Register(&runner.Test{
		Name:        "query-projection",
		Description: "Query projections by type and verify list pagination",
		Run:         runQueryProjectionTest,
	})
}

func runQueryProjectionTest(ctx context.Context, cfg *runner.Config) error {
	c := &client.Config{
		IngestionURL: cfg.IngestionURL,
		QueryURL:     cfg.QueryURL,
	}

	// Generate unique aggregate IDs for test isolation
	aggregateID1 := client.UniqueID("e2e-user-1")
	aggregateID2 := client.UniqueID("e2e-user-2")

	// 1. Ingest two user.login events
	for i, aggID := range []string{aggregateID1, aggregateID2} {
		req := &client.IngestRequest{
			EventType:   "user.login",
			AggregateID: aggID,
			Payload: map[string]interface{}{
				"user_id": aggID,
				"ip":      fmt.Sprintf("192.168.1.%d", i+1),
			},
		}

		_, err := client.IngestEvent(ctx, c, req)
		if err != nil {
			return fmt.Errorf("failed to ingest event for %s: %w", aggID, err)
		}
	}

	// 2. Wait for projections to be created
	_, err := client.WaitForProjection(ctx, c, "user_session", aggregateID1, 5*time.Second)
	if err != nil {
		return fmt.Errorf("projection 1 not created: %w", err)
	}

	_, err = client.WaitForProjection(ctx, c, "user_session", aggregateID2, 5*time.Second)
	if err != nil {
		return fmt.Errorf("projection 2 not created: %w", err)
	}

	// 3. Query single projection
	projection, err := client.GetProjection(ctx, c, "user_session", aggregateID1)
	if err != nil {
		return fmt.Errorf("failed to get projection: %w", err)
	}

	if projection == nil {
		return fmt.Errorf("expected projection, got nil")
	}

	if projection.AggregateID != aggregateID1 {
		return fmt.Errorf("expected aggregate_id %s, got %s", aggregateID1, projection.AggregateID)
	}

	// 4. List projections with pagination
	list, err := client.ListProjections(ctx, c, "user_session", 10, 0)
	if err != nil {
		return fmt.Errorf("failed to list projections: %w", err)
	}

	if list.Total < 2 {
		return fmt.Errorf("expected at least 2 projections, got %d", list.Total)
	}

	// 5. Query non-existent projection
	nonExistent, err := client.GetProjection(ctx, c, "user_session", "non-existent-id")
	if err != nil {
		return fmt.Errorf("unexpected error querying non-existent projection: %w", err)
	}

	if nonExistent != nil {
		return fmt.Errorf("expected nil for non-existent projection")
	}

	return nil
}
