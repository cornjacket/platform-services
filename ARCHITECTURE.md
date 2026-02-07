# Platform Services - Architecture

**Last Audited:** 2026-02-07

This document describes how the codebase aligns with Clean Architecture / Hexagonal (Ports and Adapters) principles.

For the directory structure, see [DEVELOPMENT.md](DEVELOPMENT.md#project-structure).

## Layer Mapping

| Clean Architecture Layer | Location | Example |
|--------------------------|----------|---------|
| **Entities (Domain)** | `internal/shared/domain/` | `events/envelope.go` |
| **Shared Domain** | `internal/shared/projections/` | `store.go`, `postgres.go` |
| **Use Cases (Application)** | `internal/services/*/service.go` | `ingestion/service.go` |
| **Ports (Interfaces)** | `internal/services/*/repository.go` | `ingestion/repository.go` |
| **Driving Adapters** | `internal/services/*/handler.go` | `ingestion/handler.go` |
| **Driven Adapters** | `internal/shared/infra/` | `postgres/outbox.go` |
| **Client Libraries** | `internal/client/` | `eventhandler/client.go` |
| **Composition Root** | `cmd/platform/main.go` | Wiring |

## Dependency Rules

### The Golden Rule

**Dependencies point inward.** Outer layers depend on inner layers, never the reverse.

```
┌─────────────────────────────────────────────────────────────┐
│  cmd/platform/main.go (Composition Root)                    │
│    ┌─────────────────────────────────────────────────────┐  │
│    │  internal/shared/infra/* (Driven Adapters)          │  │
│    │    ┌─────────────────────────────────────────────┐  │  │
│    │    │  internal/services/*/handler.go (Driving)   │  │  │
│    │    │    ┌─────────────────────────────────────┐  │  │  │
│    │    │    │  internal/services/*/service.go     │  │  │  │
│    │    │    │    ┌─────────────────────────────┐  │  │  │  │
│    │    │    │    │  internal/shared/domain/*   │  │  │  │  │
│    │    │    │    │  (Entities - NO DEPS)       │  │  │  │  │
│    │    │    │    └─────────────────────────────┘  │  │  │  │
│    │    │    └─────────────────────────────────────┘  │  │  │
│    │    └─────────────────────────────────────────────┘  │  │
│    └─────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

### Import Rules

| Package | May Import | Must NOT Import |
|---------|------------|-----------------|
| `domain/*` | stdlib only | anything in `internal/` |
| `shared/projections` | `domain/*` | `services/*`, `infra/*` |
| `services/*/service.go` | `domain/*`, `shared/projections` | `infra/*`, other services |
| `services/*/handler.go` | same service only | `infra/*`, `domain/*` directly |
| `services/*/repository.go` | `domain/*`, `shared/projections` | `infra/*` |
| `client/*` | `domain/*`, `infra/*` | `services/*` |
| `infra/*` | `domain/*`, `services/*/worker` | `services/*/service.go` |
| `cmd/platform` | everything | — |

### Current Import Verification

```
domain/events/envelope.go
    └── imports: stdlib + github.com/gofrs/uuid/v5   ✅

shared/projections/store.go
    └── imports: domain/events                       ✅

services/ingestion/service.go
    └── imports: domain/events                       ✅

services/ingestion/worker/processor.go
    └── imports: (interfaces only)                   ✅

services/eventhandler/handlers.go
    └── imports: domain/events                       ✅

services/query/service.go
    └── imports: shared/projections                  ✅

client/eventhandler/client.go
    └── imports: domain/events                       ✅

infra/postgres/outbox.go
    └── imports: domain/events, services/worker      ✅
```

## Ports and Adapters

### Ports (Interfaces Owned by Services)

Services define the interfaces they need. Infrastructure implements them.

**Example:** `internal/services/ingestion/repository.go`
```go
type OutboxRepository interface {
    Insert(ctx context.Context, event *events.Envelope) error
}
```

### Driven Adapters (Infrastructure Implements Ports)

**Example:** `internal/shared/infra/postgres/outbox.go`
```go
// OutboxRepo implements ingestion.OutboxRepository
type OutboxRepo struct { ... }

func (r *OutboxRepo) Insert(ctx context.Context, event *events.Envelope) error { ... }
```

### Driving Adapters (External → Application)

HTTP handlers translate HTTP requests into service calls.

**Example:** `internal/services/ingestion/handler.go`
```go
func (h *Handler) HandleIngest(w http.ResponseWriter, r *http.Request) {
    var req IngestRequest
    json.NewDecoder(r.Body).Decode(&req)  // translate
    resp, err := h.service.Ingest(ctx, &req)  // delegate
    h.writeJSON(w, http.StatusAccepted, resp)  // translate
}
```

## Entity Purity

Domain entities must be free of infrastructure coupling:

| Check | Status |
|-------|--------|
| No ORM decorators (`gorm:`, `db:`) | ✅ |
| No HTTP framework bindings | ✅ |
| No infrastructure imports | ✅ |
| Only stdlib + domain deps | ✅ |

## Component Testing

The client library and shared abstraction patterns enable component testing by providing clean injection points. Each service can be tested in isolation by mocking its dependencies.

### Mocking Strategy by Service

| Service | Interface to Mock | Test Scenarios |
|---------|-------------------|----------------|
| **Ingestion Worker** | `EventSubmitter` | Verify outbox processing calls `SubmitEvent()` correctly |
| **EventHandler** | `projections.Store` | Verify event handlers call `WriteProjection()` with correct state |
| **Query Service** | `ProjectionReader` | Verify `GetProjection()`/`ListProjections()` return expected data |

### Example: Testing Ingestion Worker

```go
// Mock the EventSubmitter interface
type mockSubmitter struct {
    submitted []*events.Envelope
}

func (m *mockSubmitter) SubmitEvent(ctx context.Context, e *events.Envelope) error {
    m.submitted = append(m.submitted, e)
    return nil
}

func TestProcessor_ProcessesOutboxEntry(t *testing.T) {
    mock := &mockSubmitter{}
    processor := worker.NewProcessor(outboxReader, eventStore, mock, logger)

    // ... trigger processing ...

    assert.Len(t, mock.submitted, 1)
    assert.Equal(t, "sensor.reading", mock.submitted[0].EventType)
}
```

This follows the **Dependency Inversion Principle** — high-level services depend on abstractions (interfaces), not concrete implementations. Infrastructure details (Redpanda, Postgres) are hidden behind interfaces that can be swapped for mocks in tests.

## Known Architectural Smells

### 1. DTOs in Service Layer (Low Severity)

`IngestRequest` and `IngestResponse` in `service.go` have `json` tags. Strictly, transport-specific DTOs belong in the handler layer.

**Why it's OK for now:** Pragmatic trade-off for a small codebase. The service is still testable.

### 2. Handler Depends on Concrete Service (Low Severity)

```go
type Handler struct {
    service *Service  // concrete, not interface
}
```

**Why it's OK for now:** Go idiom for internal code. Can add interface if testing requires it.

### 3. Single-Instance Ingestion Worker (Medium Severity)

The Ingestion Worker (formerly Outbox Processor) uses a dispatcher + worker pool pattern that does not support horizontal scaling across multiple instances. Running multiple instances would cause duplicate processing.

**Why it's OK for now:** Dev environment runs single instance. Throughput target is ~10 events/sec.

**Action:** See [ADR-0012](../platform-docs/decisions/0012-outbox-processing-strategy.md) for future scaling paths (row locking or SQS migration).

### 4. No Alerting for Retry Exhaustion (High Severity)

When outbox entries exhaust retries, no alert is triggered. These failures will go unnoticed until manual inspection.

**Why it's OK for now:** Dev environment with test data. Failures are visible in logs.

**Action:** Before production, implement:
- Alert on retry exhaustion
- Alarm on outbox table growth (entries not draining)

See [Task 001](tasks/001-outbox-processor.md) for rationale on why DLQ was omitted.

### 5. Non-Atomic Event Processing (Low Severity)

Event store write and outbox delete are separate operations, not in a single transaction. If delete fails after successful write, the entry will be reprocessed.

**Why it's OK for now:** Idempotency via unique constraint on `event_id` handles duplicates gracefully. Reprocessing is harmless.

**Action:** Monitor for data inconsistencies in production. If observed, consider wrapping event store write + outbox delete in a transaction (Redpanda publish would remain outside).

### 6. Single-Consumer Event Handler (Medium Severity)

The Event Handler uses a single consumer subscribing to all topics. This does not support horizontal scaling — running multiple instances would cause duplicate processing within the same consumer group.

**Why it's OK for now:** Dev environment targets ~10 events/sec. Single consumer handles this trivially.

**Action:** See [ADR-0014](../platform-docs/decisions/0014-event-handler-consumer-strategy.md) for the migration path to topic-specific consumers with partition-based parallelism.

## Adding a New Service

1. Create folder: `internal/services/<name>/`
2. Define ports: `repository.go` (interfaces the service needs)
3. Implement use cases: `service.go`
4. Add driving adapter: `handler.go` (if HTTP-exposed)
5. Add routes: `routes.go`
6. Create migrations: `migrations/*.sql`
7. Implement driven adapters in `internal/shared/infra/`
8. Wire in `cmd/platform/main.go`

## References

- [DEVELOPMENT.md](DEVELOPMENT.md) — Build patterns and conventions
- [ADR-0010](../platform-docs/decisions/0010-database-per-service-pattern.md) — Database-per-service pattern
- [Design Spec](../platform-docs/design-spec.md) — Operational parameters
