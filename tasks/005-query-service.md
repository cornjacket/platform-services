# Task 005: Query Service

**Status:** Draft
**Created:** 2026-02-06
**Updated:** 2026-02-06

## Context

Phase 1 requires the Query Service to enable automated end-to-end testing. The Query Service reads from projections and exposes them via HTTP, completing the read side of the CQRS pattern.

The API contract is defined in `api/openapi/query.yaml`.

## Functionality

- HTTP server on port 8081 (configurable via `CJ_QUERY_PORT`)
- Read projections by type and aggregate ID
- List projections by type with pagination
- Health check endpoint

## Design

### Architecture

Following the same patterns as Ingestion Service:

```
HTTP Request
    │
    ▼
Handler (handler.go)      ← HTTP routing, request/response
    │
    ▼
Service (service.go)      ← Business logic (minimal for queries)
    │
    ▼
Repository Interface      ← Defined in query package
    │
    ▼
PostgreSQL Adapter        ← Implements interface, reads from projections table
```

### Repository Interface

**internal/services/query/repository.go:**

```go
package query

import (
    "context"
    "github.com/gofrs/uuid/v5"
)

// Projection represents a projection record returned by the Query Service.
type Projection struct {
    ProjectionID       uuid.UUID       `json:"projection_id"`
    ProjectionType     string          `json:"projection_type"`
    AggregateID        string          `json:"aggregate_id"`
    State              json.RawMessage `json:"state"`
    LastEventID        uuid.UUID       `json:"last_event_id"`
    LastEventTimestamp string          `json:"last_event_timestamp"`
    UpdatedAt          string          `json:"updated_at"`
}

// ProjectionRepository defines read operations for projections.
type ProjectionRepository interface {
    // Get retrieves a single projection by type and aggregate ID.
    Get(ctx context.Context, projectionType, aggregateID string) (*Projection, error)

    // List retrieves projections by type with pagination.
    List(ctx context.Context, projectionType string, limit, offset int) ([]Projection, int, error)
}
```

### Endpoints

Per `api/openapi/query.yaml`:

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/projections/{projection_type}/{aggregate_id}` | Get single projection |
| GET | `/api/v1/projections/{projection_type}` | List projections by type |
| GET | `/health` | Health check |

### Error Handling

| Condition | HTTP Status | Response |
|-----------|-------------|----------|
| Projection not found | 404 | `{"error": "projection not found"}` |
| Invalid projection type | 400 | `{"error": "invalid projection type"}` |
| Database error | 500 | `{"error": "internal server error"}` |

### Database

Query Service connects to Event Handler's database (same database in dev, separate in prod per ADR-0010).

Uses `CJ_QUERY_DATABASE_URL` which defaults to the shared local database.

## Files to Create

### New Files

- `internal/services/query/repository.go` — Interface and types
- `internal/services/query/service.go` — Business logic
- `internal/services/query/handler.go` — HTTP handlers
- `internal/services/query/routes.go` — Route registration
- `internal/shared/infra/postgres/query_projections.go` — Repository implementation

### Modify

- `cmd/platform/main.go` — Initialize and start Query Service

## Acceptance Criteria

- [ ] GET `/api/v1/projections/{type}/{id}` returns projection or 404
- [ ] GET `/api/v1/projections/{type}` returns paginated list
- [ ] GET `/health` returns `{"status": "healthy"}`
- [ ] Query Service starts on port 8081 (or `CJ_QUERY_PORT`)
- [ ] Follows repository pattern (interface in query package, implementation in infra)
- [ ] Structured logging with slog
- [ ] API matches `api/openapi/query.yaml` contract

## Testing

After implementation, verify with:

```bash
# Start services
docker compose up -d
make migrate-all
go run ./cmd/platform

# Ingest an event
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"event_type":"sensor.reading","aggregate_id":"device-001","payload":{"value":72.5,"unit":"fahrenheit"}}'

# Wait for processing
sleep 2

# Query the projection via Query Service
curl http://localhost:8081/api/v1/projections/sensor_state/device-001

# List all sensor_state projections
curl http://localhost:8081/api/v1/projections/sensor_state

# Health check
curl http://localhost:8081/health
```

## Notes

- The Query Service is read-only; it never writes to the database
- Projection types are currently: `sensor_state`, `user_session`
- Future: Add caching layer if read performance becomes a concern
