# Task 007: Service Client Libraries

**Status:** In Progress
**Created:** 2026-02-06
**Updated:** 2026-02-06

## Context

Currently, services communicate through shared infrastructure (Postgres, Redpanda) with implementation details scattered across packages. This makes component testing difficult because there's no clear boundary to mock.

This task restructures inter-service communication to use client libraries, enabling:
1. **Component testing** — Mock the client to test each service in isolation
2. **Clear ownership** — Each service provides/consumes well-defined clients
3. **Microservice readiness** — Clients can be moved to `pkg/` when extracting services

## Goals

1. Move outbox under ingestion (it's Ingestion's internal worker pool)
2. Create `client/eventhandler` for submitting events to EventHandler
3. Create `shared/projections` for projection read/write access
4. Update services to use these abstractions

## Architecture

### High-Level Service View

```
┌─────────────┐         ┌──────────────┐         ┌─────────────┐
│  Ingestion  │────────▶│ EventHandler │────────▶│    Query    │
│   Service   │  events │   Service    │  state  │   Service   │
└─────────────┘         └──────────────┘         └─────────────┘
```

Three services. Events flow left to right. Infrastructure details are hidden.

### Data Flow (Detailed)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           INGESTION SERVICE                                 │
│                                                                             │
│   ┌──────────────┐     ┌──────────────┐     ┌──────────────────────────┐   │
│   │ HTTP Handler │────▶│ Outbox Table │◀────│ Worker Pool              │   │
│   │              │     │              │     │                          │   │
│   │ 1. Validate  │     │ (internal)   │     │ 3. Read from outbox      │   │
│   │ 2. Write     │     │              │     │ 4. Write to Event Store  │   │
│   └──────────────┘     └──────────────┘     │ 5. SubmitEvent() ────────┼───┼──┐
│                                             │ 6. Delete from outbox    │   │  │
│                                             └──────────────────────────┘   │  │
└─────────────────────────────────────────────────────────────────────────────┘  │
                                                                                  │
         ┌────────────────────────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────┐
│ client/eventhandler │
│                     │
│ SubmitEvent()       │──────▶ Redpanda ──────▶ EventHandler
│ (wraps Redpanda)    │
└─────────────────────┘
```

### Client Libraries

| Client | Methods | Used By | Wraps |
|--------|---------|---------|-------|
| `client/eventhandler` | `SubmitEvent(event)` | Ingestion Worker | Redpanda publish |
| `shared/projections` | `WriteProjection()`, `GetProjection()`, `ListProjections()` | EventHandler, Query | Postgres projections |

## Design

### 1. Move Outbox Under Ingestion

The outbox is Ingestion's internal mechanism for reliable event delivery. The worker pool (currently `services/outbox`) is part of Ingestion, not a separate service.

**Current:**
```
internal/services/
├── ingestion/
├── outbox/              ← Sibling (incorrect)
```

**New:**
```
internal/services/
├── ingestion/
│   ├── handler.go
│   ├── service.go
│   ├── repository.go
│   ├── routes.go
│   └── worker/          ← Sub-package (correct)
│       ├── processor.go
│       └── repository.go
```

### 2. EventHandler Client

Provides `SubmitEvent()` for Ingestion to send events to EventHandler. Wraps Redpanda publish internally.

```go
// internal/client/eventhandler/client.go
package eventhandler

type Client struct {
    producer EventPublisher
    logger   *slog.Logger
}

type EventPublisher interface {
    Publish(ctx context.Context, topic string, event *events.Envelope) error
}

func New(producer EventPublisher, logger *slog.Logger) *Client { ... }

func (c *Client) SubmitEvent(ctx context.Context, event *events.Envelope) error {
    topic := topicFromEventType(event.EventType)
    return c.producer.Publish(ctx, topic, event)
}
```

Ingestion's worker will depend on an interface that this client satisfies:

```go
// internal/services/ingestion/worker/repository.go
type EventSubmitter interface {
    SubmitEvent(ctx context.Context, event *events.Envelope) error
}
```

### 3. Shared Projections Package

Both EventHandler and Query need access to projections. This is genuinely shared data (the boundary between write and read sides in CQRS).

```go
// internal/shared/projections/repository.go
package projections

type Store interface {
    WriteProjection(ctx context.Context, projType, aggregateID string,
                    state []byte, event *events.Envelope) error
    GetProjection(ctx context.Context, projType, aggregateID string) (*Projection, error)
    ListProjections(ctx context.Context, projType string, limit, offset int) ([]Projection, int, error)
}

type Projection struct {
    ProjectionID       uuid.UUID
    ProjectionType     string
    AggregateID        string
    State              json.RawMessage
    LastEventID        uuid.UUID
    LastEventTimestamp time.Time
    UpdatedAt          time.Time
}
```

```go
// internal/shared/projections/postgres.go
package projections

type PostgresStore struct {
    pool   *pgxpool.Pool
    logger *slog.Logger
}

func NewPostgresStore(pool *pgxpool.Pool, logger *slog.Logger) *PostgresStore { ... }

func (s *PostgresStore) WriteProjection(...) error { ... }
func (s *PostgresStore) GetProjection(...) (*Projection, error) { ... }
func (s *PostgresStore) ListProjections(...) ([]Projection, int, error) { ... }
```

### 4. Service Updates

**EventHandler:**
- Remove internal `ProjectionRepository`
- Use `shared/projections.Store` interface
- Handlers call `store.WriteProjection()`

**Query Service:**
- Remove internal `ProjectionRepository`
- Use `shared/projections.Store` interface
- Service calls `store.GetProjection()`, `store.ListProjections()`

**Ingestion Worker:**
- Replace direct Redpanda publish with `EventSubmitter` interface
- Inject `client/eventhandler.Client` at startup

## Files to Create/Modify

### Create

| File | Description |
|------|-------------|
| `internal/client/eventhandler/client.go` | EventHandler client with `SubmitEvent()` |
| `internal/shared/projections/repository.go` | `Store` interface and `Projection` type |
| `internal/shared/projections/postgres.go` | PostgreSQL implementation |
| `internal/services/ingestion/worker/processor.go` | Moved from `services/outbox/` |
| `internal/services/ingestion/worker/repository.go` | Moved from `services/outbox/` |

### Modify

| File | Changes |
|------|---------|
| `internal/services/eventhandler/handlers.go` | Use `projections.Store` interface |
| `internal/services/eventhandler/repository.go` | Remove `ProjectionRepository`, add `ProjectionWriter` interface |
| `internal/services/query/service.go` | Use `projections.Store` interface |
| `internal/services/query/repository.go` | Remove `ProjectionRepository` |
| `internal/shared/infra/postgres/projections.go` | Remove (merged into `shared/projections/`) |
| `internal/shared/infra/postgres/query_projections.go` | Remove (merged into `shared/projections/`) |
| `cmd/platform/main.go` | Update wiring for new structure |

### Delete

| File | Reason |
|------|--------|
| `internal/services/outbox/` | Moved to `internal/services/ingestion/worker/` |

## Documentation Updates

Update documentation to reflect the new architecture. High-level diagrams should emphasize the 3-service view (Ingestion, EventHandler, Query) and hide infrastructure details.

### platform-services

| Document | Updates |
|----------|---------|
| `ARCHITECTURE.md` | Update layer mapping, dependency rules, import verification examples |
| `DEVELOPMENT.md` | Update project structure to show `ingestion/worker/`, `client/`, `shared/projections/` |

### platform-docs

| Document | Updates |
|----------|---------|
| `design-spec.md` | Simplify data flow diagrams to show service-level view; move infrastructure details to implementation notes |
| `README.md` | Update if directory structure is referenced |

### Key Documentation Principles

1. **Top-level diagrams** show 3 services: Ingestion → EventHandler → Query
2. **Infrastructure details** (outbox, Redpanda, projections table) are implementation notes, not primary diagrams
3. **Client libraries** are documented as the interface between services

## Acceptance Criteria

### Code Changes
- [ ] Outbox processor moved to `internal/services/ingestion/worker/`
- [ ] `client/eventhandler` package created with `SubmitEvent()`
- [ ] `shared/projections` package created with `Store` interface
- [ ] EventHandler uses `shared/projections.Store` for writes
- [ ] Query Service uses `shared/projections.Store` for reads
- [ ] Ingestion Worker uses `EventSubmitter` interface (satisfied by eventhandler client)
- [ ] All existing tests pass
- [ ] E2E tests pass

### Documentation Updates
- [ ] `ARCHITECTURE.md` updated to reflect new structure
- [ ] `DEVELOPMENT.md` project structure updated
- [ ] `design-spec.md` diagrams simplified to service-level view
- [ ] All documentation consistent with 3-service architecture (Ingestion, EventHandler, Query)

## Component Testing Enabled

After this restructuring, each service can be tested in isolation:

| Service | Mock | Test Scenarios |
|---------|------|----------------|
| Ingestion | `EventSubmitter` | Validate → Outbox → SubmitEvent called correctly |
| EventHandler | `projections.Store` | Event received → WriteProjection called with correct state |
| Query | `projections.Store` | GetProjection/ListProjections return expected data |

## Notes

- The `topicFromEventType()` function moves to `client/eventhandler/` since it's part of the event submission logic
- Event Store write remains in Ingestion Worker (it's an audit log, not inter-service communication)
- Redpanda producer implementation stays in `infra/redpanda/`, client just wraps it
