# Platform Services - Architecture

**Last Audited:** 2026-02-05

This document describes how the codebase aligns with Clean Architecture / Hexagonal (Ports and Adapters) principles.

## Folder Structure

```
platform-services/
├── cmd/
│   └── platform/
│       └── main.go                    # Composition Root (wiring)
│
├── internal/
│   ├── shared/
│   │   ├── config/
│   │   │   └── config.go              # Configuration loading
│   │   ├── domain/
│   │   │   ├── events/
│   │   │   │   └── envelope.go        # Domain model
│   │   │   └── models/                # Domain models (future)
│   │   └── infra/
│   │       ├── http/                  # HTTP client adapters (future)
│   │       ├── postgres/
│   │       │   ├── client.go          # DB connection adapter
│   │       │   └── outbox.go          # OutboxRepository implementation
│   │       └── redpanda/              # Kafka producer/consumer (future)
│   │
│   └── services/
│       ├── ingestion/
│       │   ├── handler.go             # HTTP handler (driving adapter)
│       │   ├── service.go             # Application/Use Case logic
│       │   ├── repository.go          # Port definition (interface)
│       │   ├── routes.go              # Route registration
│       │   └── migrations/            # Service-owned migrations
│       │
│       ├── eventhandler/              # Event Handler service (future)
│       ├── actions/                   # Action Orchestrator (future)
│       ├── outbox/                    # Outbox Processor (future)
│       ├── query/                     # Query Service (future)
│       └── tsdb/                      # TSDB Writer (future)
│
├── api/openapi/                       # API definitions (future)
├── deploy/terraform/                  # Deployment code (future)
└── pkg/client/                        # Public Go SDK (future)
```

## Layer Mapping

| Clean Architecture Layer | Location | Example |
|--------------------------|----------|---------|
| **Entities (Domain)** | `internal/shared/domain/` | `events/envelope.go` |
| **Use Cases (Application)** | `internal/services/*/service.go` | `ingestion/service.go` |
| **Ports (Interfaces)** | `internal/services/*/repository.go` | `ingestion/repository.go` |
| **Driving Adapters** | `internal/services/*/handler.go` | `ingestion/handler.go` |
| **Driven Adapters** | `internal/shared/infra/` | `postgres/outbox.go` |
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
| `services/*/service.go` | `domain/*` | `infra/*`, other services |
| `services/*/handler.go` | same service only | `infra/*`, `domain/*` directly |
| `services/*/repository.go` | `domain/*` | `infra/*` |
| `infra/*` | `domain/*` | `services/*` |
| `cmd/platform` | everything | — |

### Current Import Verification

```
domain/events/envelope.go
    └── imports: stdlib + github.com/gofrs/uuid/v5   ✅

services/ingestion/service.go
    └── imports: domain/events                       ✅

services/ingestion/repository.go
    └── imports: domain/events                       ✅

services/ingestion/handler.go
    └── imports: (nothing external)                  ✅

infra/postgres/outbox.go
    └── imports: domain/events                       ✅
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

## Event Types

Events flow through the system via the Outbox Processor → Redpanda → Event Handler pipeline. The `event_type` field determines topic routing and projection handling.

### Topic Routing

| Prefix | Topic | Description |
|--------|-------|-------------|
| `sensor.*` | sensor-events | IoT sensor data |
| `user.*` | user-actions | User activity |
| `*` (default) | system-events | System/operational events |

### Initial Event Catalog

| Event Type | Payload Schema | Projection |
|------------|----------------|------------|
| `sensor.reading` | `{"value": float, "unit": string}` | `sensor_state` |
| `user.login` | `{"user_id": string, "ip": string}` | `user_session` |
| `system.alert` | `{"level": string, "message": string}` | (none) |

### Example Events

```json
// sensor.reading — aggregate_id is the device
{"event_type": "sensor.reading", "aggregate_id": "device-001", "payload": {"value": 72.5, "unit": "fahrenheit"}}

// user.login — aggregate_id is the user
{"event_type": "user.login", "aggregate_id": "user-123", "payload": {"user_id": "user-123", "ip": "192.168.1.1"}}

// system.alert — aggregate_id is the source component
{"event_type": "system.alert", "aggregate_id": "cluster-1", "payload": {"level": "warn", "message": "High memory usage"}}
```

### Projections

| Projection Type | Purpose | Updated By |
|-----------------|---------|------------|
| `sensor_state` | Latest sensor reading per device | `sensor.reading` |
| `user_session` | Last login info per user | `user.login` |

New event types and projections are added as features require them.

## Entity Purity

Domain entities must be free of infrastructure coupling:

| Check | Status |
|-------|--------|
| No ORM decorators (`gorm:`, `db:`) | ✅ |
| No HTTP framework bindings | ✅ |
| No infrastructure imports | ✅ |
| Only stdlib + domain deps | ✅ |

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

### 3. OutboxEntry Defined in Infrastructure (Medium Severity)

`OutboxEntry` in `postgres/outbox.go` will be needed by the Outbox Processor service. May need to move to domain or become a port type.

**Action:** Address when implementing Outbox Processor.

### 4. Single-Instance Outbox Processor (Medium Severity)

The Outbox Processor uses a dispatcher + worker pool pattern that does not support horizontal scaling across multiple instances. Running multiple instances would cause duplicate processing.

**Why it's OK for now:** Dev environment runs single instance. Throughput target is ~10 events/sec.

**Action:** See [ADR-0012](../platform-docs/decisions/0012-outbox-processing-strategy.md) for future scaling paths (row locking or SQS migration).

### 5. No Alerting for Retry Exhaustion (High Severity)

When outbox entries exhaust retries, no alert is triggered. These failures will go unnoticed until manual inspection.

**Why it's OK for now:** Dev environment with test data. Failures are visible in logs.

**Action:** Before production, implement:
- Alert on retry exhaustion
- Alarm on outbox table growth (entries not draining)

See [Task 001](tasks/001-outbox-processor.md) for rationale on why DLQ was omitted.

### 6. Non-Atomic Outbox Processing (Low Severity)

Event store write and outbox delete are separate operations, not in a single transaction. If delete fails after successful write, the entry will be reprocessed.

**Why it's OK for now:** Idempotency via unique constraint on `event_id` handles duplicates gracefully. Reprocessing is harmless.

**Action:** Monitor for data inconsistencies in production. If observed, consider wrapping event store write + outbox delete in a transaction (Redpanda publish would remain outside).

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
