package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config holds client configuration.
type Config struct {
	IngestionURL string
	QueryURL     string
}

// IngestRequest represents a request to the ingestion API.
type IngestRequest struct {
	EventType   string      `json:"event_type"`
	AggregateID string      `json:"aggregate_id"`
	Payload     interface{} `json:"payload"`
}

// IngestResponse represents the response from the ingestion API.
type IngestResponse struct {
	EventID string `json:"event_id"`
	Status  string `json:"status"`
}

// Projection represents a projection from the query API.
type Projection struct {
	ProjectionID       string          `json:"projection_id"`
	ProjectionType     string          `json:"projection_type"`
	AggregateID        string          `json:"aggregate_id"`
	State              json.RawMessage `json:"state"`
	LastEventID        string          `json:"last_event_id"`
	LastEventTimestamp string          `json:"last_event_timestamp"`
	UpdatedAt          string          `json:"updated_at"`
}

// ProjectionList represents a list of projections from the query API.
type ProjectionList struct {
	Projections []Projection `json:"projections"`
	Total       int          `json:"total"`
	Limit       int          `json:"limit"`
	Offset      int          `json:"offset"`
}

// ErrorResponse represents an error response from the API.
type ErrorResponse struct {
	Error string `json:"error"`
}

// UniqueID generates a unique ID for test isolation.
func UniqueID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// IngestEvent posts an event to the ingestion API.
func IngestEvent(ctx context.Context, cfg *Config, req *IngestRequest) (*IngestResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.IngestionURL+"/api/v1/events", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		var errResp ErrorResponse
		json.Unmarshal(respBody, &errResp)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, errResp.Error)
	}

	var ingestResp IngestResponse
	if err := json.Unmarshal(respBody, &ingestResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &ingestResp, nil
}

// GetProjection retrieves a projection from the query API.
func GetProjection(ctx context.Context, cfg *Config, projectionType, aggregateID string) (*Projection, error) {
	url := fmt.Sprintf("%s/api/v1/projections/%s/%s", cfg.QueryURL, projectionType, aggregateID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Not found is not an error
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		json.Unmarshal(respBody, &errResp)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, errResp.Error)
	}

	var projection Projection
	if err := json.Unmarshal(respBody, &projection); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &projection, nil
}

// ListProjections retrieves a list of projections from the query API.
func ListProjections(ctx context.Context, cfg *Config, projectionType string, limit, offset int) (*ProjectionList, error) {
	url := fmt.Sprintf("%s/api/v1/projections/%s?limit=%d&offset=%d", cfg.QueryURL, projectionType, limit, offset)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		json.Unmarshal(respBody, &errResp)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, errResp.Error)
	}

	var list ProjectionList
	if err := json.Unmarshal(respBody, &list); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &list, nil
}

// WaitForProjection polls for a projection until it appears or timeout.
func WaitForProjection(ctx context.Context, cfg *Config, projectionType, aggregateID string, timeout time.Duration) (*Projection, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		projection, err := GetProjection(ctx, cfg, projectionType, aggregateID)
		if err != nil {
			return nil, err
		}
		if projection != nil {
			return projection, nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return nil, fmt.Errorf("timeout waiting for projection %s/%s", projectionType, aggregateID)
}

// CheckHealth checks the health endpoint of a service.
func CheckHealth(ctx context.Context, url string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/health", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}
