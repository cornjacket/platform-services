package query

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gofrs/uuid/v5"

	"github.com/cornjacket/platform-services/internal/shared/projections"
)

func TestGetProjection_Success(t *testing.T) {
	expected := &projections.Projection{
		ProjectionID:       uuid.Must(uuid.NewV7()),
		ProjectionType:     "sensor_state",
		AggregateID:        "device-001",
		State:              json.RawMessage(`{"temperature": 72.5}`),
		LastEventID:        uuid.Must(uuid.NewV7()),
		LastEventTimestamp: time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC),
	}

	mock := &mockProjectionReader{
		GetProjectionFn: func(ctx context.Context, projType, aggregateID string) (*projections.Projection, error) {
			return expected, nil
		},
	}
	service := NewService(mock, slog.Default())

	result, err := service.GetProjection(context.Background(), "sensor_state", "device-001")
	require.NoError(t, err)
	assert.Equal(t, "device-001", result.AggregateID)
	assert.Equal(t, "sensor_state", result.ProjectionType)
}

func TestGetProjection_InvalidType(t *testing.T) {
	mock := &mockProjectionReader{
		GetProjectionFn: func(ctx context.Context, projType, aggregateID string) (*projections.Projection, error) {
			t.Fatal("store should not be called for invalid type")
			return nil, nil
		},
	}
	service := NewService(mock, slog.Default())

	_, err := service.GetProjection(context.Background(), "invalid_type", "device-001")
	assert.Error(t, err)
}

func TestGetProjection_StoreError(t *testing.T) {
	mock := &mockProjectionReader{
		GetProjectionFn: func(ctx context.Context, projType, aggregateID string) (*projections.Projection, error) {
			return nil, fmt.Errorf("no rows in result set")
		},
	}
	service := NewService(mock, slog.Default())

	_, err := service.GetProjection(context.Background(), "sensor_state", "nonexistent")
	assert.Error(t, err)
}

func TestListProjections_Success(t *testing.T) {
	storeResults := []projections.Projection{
		{
			ProjectionID:       uuid.Must(uuid.NewV7()),
			ProjectionType:     "sensor_state",
			AggregateID:        "device-001",
			State:              json.RawMessage(`{}`),
			LastEventID:        uuid.Must(uuid.NewV7()),
			LastEventTimestamp: time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC),
			UpdatedAt:          time.Date(2026, 2, 9, 12, 0, 0, 0, time.UTC),
		},
	}

	mock := &mockProjectionReader{
		ListProjectionsFn: func(ctx context.Context, projType string, limit, offset int) ([]projections.Projection, int, error) {
			return storeResults, 1, nil
		},
	}
	service := NewService(mock, slog.Default())

	result, err := service.ListProjections(context.Background(), "sensor_state", 20, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	assert.Len(t, result.Projections, 1)
}

func TestListProjections_PaginationDefaults(t *testing.T) {
	var capturedLimit, capturedOffset int
	mock := &mockProjectionReader{
		ListProjectionsFn: func(ctx context.Context, projType string, limit, offset int) ([]projections.Projection, int, error) {
			capturedLimit = limit
			capturedOffset = offset
			return nil, 0, nil
		},
	}
	service := NewService(mock, slog.Default())

	_, err := service.ListProjections(context.Background(), "sensor_state", 0, 0)
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit, "zero limit should default to 20")

	_, err = service.ListProjections(context.Background(), "sensor_state", 10, -5)
	require.NoError(t, err)
	assert.Equal(t, 0, capturedOffset, "negative offset should clamp to 0")
}

func TestListProjections_LimitCapping(t *testing.T) {
	var capturedLimit int
	mock := &mockProjectionReader{
		ListProjectionsFn: func(ctx context.Context, projType string, limit, offset int) ([]projections.Projection, int, error) {
			capturedLimit = limit
			return nil, 0, nil
		},
	}
	service := NewService(mock, slog.Default())

	_, err := service.ListProjections(context.Background(), "sensor_state", 500, 0)
	require.NoError(t, err)
	assert.Equal(t, 100, capturedLimit, "limit above 100 should be capped")
}

func TestListProjections_InvalidType(t *testing.T) {
	mock := &mockProjectionReader{
		ListProjectionsFn: func(ctx context.Context, projType string, limit, offset int) ([]projections.Projection, int, error) {
			t.Fatal("store should not be called for invalid type")
			return nil, 0, nil
		},
	}
	service := NewService(mock, slog.Default())

	_, err := service.ListProjections(context.Background(), "invalid_type", 20, 0)
	assert.Error(t, err)
}
