package query

import (
	"context"
	"fmt"
	"log/slog"
)

// Valid projection types
var validProjectionTypes = map[string]bool{
	"sensor_state": true,
	"user_session": true,
}

// Service handles query business logic.
type Service struct {
	repo   ProjectionRepository
	logger *slog.Logger
}

// NewService creates a new query service.
func NewService(repo ProjectionRepository, logger *slog.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger.With("service", "query"),
	}
}

// GetProjection retrieves a projection by type and aggregate ID.
func (s *Service) GetProjection(ctx context.Context, projectionType, aggregateID string) (*Projection, error) {
	if !validProjectionTypes[projectionType] {
		return nil, fmt.Errorf("invalid projection type: %s", projectionType)
	}

	projection, err := s.repo.Get(ctx, projectionType, aggregateID)
	if err != nil {
		s.logger.Error("failed to get projection",
			"projection_type", projectionType,
			"aggregate_id", aggregateID,
			"error", err,
		)
		return nil, err
	}

	return projection, nil
}

// ListProjections retrieves projections by type with pagination.
func (s *Service) ListProjections(ctx context.Context, projectionType string, limit, offset int) (*ProjectionList, error) {
	if !validProjectionTypes[projectionType] {
		return nil, fmt.Errorf("invalid projection type: %s", projectionType)
	}

	// Apply defaults and limits
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	projections, total, err := s.repo.List(ctx, projectionType, limit, offset)
	if err != nil {
		s.logger.Error("failed to list projections",
			"projection_type", projectionType,
			"limit", limit,
			"offset", offset,
			"error", err,
		)
		return nil, err
	}

	return &ProjectionList{
		Projections: projections,
		Total:       total,
		Limit:       limit,
		Offset:      offset,
	}, nil
}

// IsValidProjectionType checks if a projection type is valid.
func IsValidProjectionType(projectionType string) bool {
	return validProjectionTypes[projectionType]
}
