package query

import (
	"context"

	"github.com/cornjacket/platform-services/internal/shared/projections"
)

// mockProjectionReader implements ProjectionReader for testing.
type mockProjectionReader struct {
	GetProjectionFn  func(ctx context.Context, projType, aggregateID string) (*projections.Projection, error)
	ListProjectionsFn func(ctx context.Context, projType string, limit, offset int) ([]projections.Projection, int, error)
}

func (m *mockProjectionReader) GetProjection(ctx context.Context, projType, aggregateID string) (*projections.Projection, error) {
	return m.GetProjectionFn(ctx, projType, aggregateID)
}

func (m *mockProjectionReader) ListProjections(ctx context.Context, projType string, limit, offset int) ([]projections.Projection, int, error) {
	return m.ListProjectionsFn(ctx, projType, limit, offset)
}
