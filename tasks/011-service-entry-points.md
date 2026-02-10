# Spec 011: Service Entry Points

**Type:** Spec
**Status:** Complete
**Created:** 2026-02-10
**Updated:** 2026-02-10

## Context

The current `cmd/platform/main.go` (238 lines) contains all service wiring: it creates DB pools, instantiates repos, builds services, registers HTTP routes, starts servers, starts workers, and manages shutdown. Every internal detail of every service is exposed in `main.go`.

This creates three problems:

1. **No encapsulation.** Adding a new handler to the ingestion service means editing `main.go`, not the ingestion package. The service boundary is not enforced by code structure.
2. **Component testing is impossible.** There's no way to start a single service in a test with real infrastructure but without the other services. The wiring logic lives in `main()`, not in a callable function.
3. **Containerization requires duplication.** When services are deployed as separate containers (Phase 4), each needs its own `main.go`. Without a `Service()` entry point, each container's `main.go` would duplicate the wiring logic currently inlined in the monolith's `main()`.

## Functionality

Introduce a `Service()` function in each service package that encapsulates all internal wiring. `cmd/platform/main.go` becomes a thin orchestrator that creates shared resources (DB pools, logger) and calls each `Service()` function.

Each `Service()` function:
- Takes a service-specific config struct
- Takes a `*pgxpool.Pool` as a concrete dependency where the service owns its DB wiring internally
- Takes service outputs as interfaces (for testability and component testing)
- Takes a logger
- Owns all internal wiring: creating repos, services, handlers, routes, servers
- Returns a `Shutdown()` function or similar mechanism for graceful teardown

## Design

### New Files

Each service gets a new entry point file:

| File | Function |
|------|----------|
| `internal/services/ingestion/ingestion.go` | `func Service(cfg Config, pool *pgxpool.Pool, submitter EventSubmitter, logger *slog.Logger) (*RunningService, error)` |
| `internal/services/eventhandler/eventhandler.go` | `func Service(cfg Config, writer ProjectionWriter, logger *slog.Logger) (*RunningService, error)` |
| `internal/services/query/query.go` | `func Service(cfg Config, pool *pgxpool.Pool, logger *slog.Logger) (*RunningService, error)` |

### Service Function Signatures

**Ingestion** — the most complex service. It runs both an HTTP server and the outbox worker.

```go
// ingestion.go

type Config struct {
    Port           int
    WorkerCount    int
    BatchSize      int
    MaxRetries     int
    PollInterval   time.Duration
    DatabaseURL    string        // needed for dedicated LISTEN connection
}

// EventSubmitter is the external dependency — publishes events for downstream processing.
// Satisfied by client/eventhandler.Client in production, mockable in tests.
type EventSubmitter interface {
    SubmitEvent(ctx context.Context, event *events.Envelope) error
}

type RunningService struct {
    Shutdown func(ctx context.Context) error
}

func Service(ctx context.Context, cfg Config, pool *pgxpool.Pool, submitter EventSubmitter, logger *slog.Logger) (*RunningService, error)
```

Internally, `Service()` creates:
- `OutboxRepo` from `pool`
- `EventStoreRepo` from `pool`
- `OutboxReaderAdapter` from `pool`
- Dedicated `pgx.Conn` for LISTEN (from `cfg.DatabaseURL`)
- `ingestion.Service` → `Handler` → `http.ServeMux` → `http.Server`
- `worker.Processor`
- Starts HTTP server and worker in goroutines
- Returns `RunningService` with a `Shutdown` that stops the HTTP server and cancels the worker

**EventHandler** — consumes from Redpanda, writes to projections.

```go
// eventhandler.go

type Config struct {
    Brokers       []string
    ConsumerGroup string
    Topics        []string
    PollTimeout   time.Duration
}

// ProjectionWriter is the service's output dependency — writes projections for downstream consumers.
// Satisfied by projections.PostgresStore in production, mockable in component tests.
// Already defined in repository.go; reused here, not duplicated.

type RunningService struct {
    Shutdown func(ctx context.Context) error
}

func Service(ctx context.Context, cfg Config, writer ProjectionWriter, logger *slog.Logger) (*RunningService, error)
```

Internally, `Service()` creates:
- `HandlerRegistry` with `SensorHandler` and `UserHandler` (using injected `writer`)
- `Consumer` with config
- Starts consumer in goroutine
- Returns `RunningService` with a `Shutdown` that closes the consumer

Note: EventHandler does not take a `*pgxpool.Pool`. The pool was only used to create `PostgresStore`, which is now injected as `ProjectionWriter`. `main.go` creates the store from the pool and passes it in.

**Query** — HTTP server, read-only.

```go
// query.go

type Config struct {
    Port int
}

type RunningService struct {
    Shutdown func(ctx context.Context) error
}

func Service(ctx context.Context, cfg Config, pool *pgxpool.Pool, logger *slog.Logger) (*RunningService, error)
```

Internally, `Service()` creates:
- `projections.PostgresStore` from `pool`
- `query.Service` → `Handler` → `http.ServeMux` → `http.Server`
- Starts HTTP server in goroutine
- Returns `RunningService` with a `Shutdown` that stops the HTTP server

### Interface Boundaries

Each `Service()` function takes external dependencies as interfaces:

| Service | Concrete DB | Interface Dependencies | Why Interface |
|---------|-------------|----------------------|---------------|
| Ingestion | `*pgxpool.Pool` | `EventSubmitter` | Service output — publishes events to Redpanda for EventHandler. Mockable to verify what was published. |
| EventHandler | (none) | `ProjectionWriter` | Service output — writes projections consumed by Query. Mockable to verify what was written. |
| Query | `*pgxpool.Pool` | (none) | Service output is HTTP responses to external clients — no downstream service to mock. |

**The rule: inject outputs, not inputs.** Service inputs (HTTP requests, Redpanda messages) arrive via infrastructure that the service configures internally. Service outputs cross to the next service in the pipeline and must be injectable for component testing.

The DB pool is concrete where used (Ingestion, Query) because:
- It's a resource, not a behavior — you don't mock a connection pool
- Services create their own repos from the pool (the repos are the abstraction layer)
- Per ADR-0010, each service gets its own pool in production

EventHandler doesn't take a pool because its only DB interaction (projection writes) is its output — already covered by the `ProjectionWriter` interface.

### RunningService Pattern

All three `Service()` functions return `*RunningService` with a `Shutdown` method. This gives `main.go` a uniform way to manage lifecycle:

```go
ingestion, err := ingestion.Service(ctx, ingestionCfg, ingestionPool, submitter, logger)
// ...
eventHandler, err := eventhandler.Service(ctx, ehCfg, projectionsStore, logger)
// ...
querySvc, err := query.Service(ctx, queryCfg, queryPool, logger)
// ...

// Shutdown in reverse order
querySvc.Shutdown(shutdownCtx)
eventHandler.Shutdown(shutdownCtx)
ingestion.Shutdown(shutdownCtx)
```

### What main.go Becomes

After refactoring, `cmd/platform/main.go` handles only:

1. Load global config (`config.Load()`)
2. Create logger
3. Create DB pools (Ingestion, EventHandler, Query)
4. Create shared external resources (Redpanda producer → `eventhandler.Client`, `projections.PostgresStore` from EventHandler pool)
5. Call each `Service()` function, passing pools and output interfaces
6. Wait for OS signal
7. Call `Shutdown()` on each service

All service-internal wiring (repos, handlers, routes, servers) moves into the respective `Service()` functions. `main.go` should drop from ~238 lines to ~80-100 lines.

### Existing Interfaces Stay Unchanged

The interfaces defined in each service's `repository.go` (`OutboxRepository`, `OutboxReader`, `EventStoreWriter`, `ProjectionWriter`, `ProjectionReader`, `EventHandler`, `EventConsumer`) are not modified. `ProjectionWriter` (already defined in `eventhandler/repository.go`) is promoted from an internal dependency to the `Service()` function signature — it becomes the output interface. No new interface definitions are needed.

### Config Struct Relationship

The new per-service `Config` structs contain a subset of the current `shared/config.Config` fields. The shared `Config` struct stays — `main.go` still loads it, then maps fields to the per-service configs:

```go
ingestionCfg := ingestion.Config{
    Port:         cfg.PortIngestion,
    WorkerCount:  cfg.OutboxWorkerCount,
    BatchSize:    cfg.OutboxBatchSize,
    MaxRetries:   cfg.OutboxMaxRetries,
    PollInterval: cfg.OutboxPollInterval,
    DatabaseURL:  cfg.DatabaseURLIngestion,
}
```

This avoids coupling service packages to the global config struct.

## Files to Create/Modify

### New Files

| File | Purpose |
|------|---------|
| `internal/services/ingestion/ingestion.go` | `Service()` entry point + `Config` + `RunningService` |
| `internal/services/eventhandler/eventhandler.go` | `Service()` entry point + `Config` + `RunningService` |
| `internal/services/query/query.go` | `Service()` entry point + `Config` + `RunningService` |

### Modified Files

| File | Change |
|------|--------|
| `cmd/platform/main.go` | Replace inline wiring with `Service()` calls. Drop from ~238 to ~80-100 lines. |

### No Changes Required

| File | Why |
|------|-----|
| `internal/services/*/repository.go` | Existing interfaces unchanged |
| `internal/services/*/handler.go` | Handlers unchanged, just wired differently |
| `internal/services/*/service.go` | Business logic unchanged |
| `internal/services/*/routes.go` | Route registration unchanged |
| `internal/shared/config/config.go` | Global config stays; per-service configs are new |
| `internal/shared/infra/postgres/*` | Infrastructure adapters unchanged |
| `internal/shared/infra/redpanda/*` | Producer unchanged |
| All unit tests | No production API changes |
| All integration tests | No production API changes |

## Acceptance Criteria

- [x] `internal/services/ingestion/ingestion.go` defines `Config`, `RunningService`, and `func Start()`
- [x] `internal/services/eventhandler/eventhandler.go` defines `Config`, `RunningService`, and `func Start()`
- [x] `internal/services/query/query.go` defines `Config`, `RunningService`, and `func Start()`
- [x] Ingestion and Query `Start()` take `*pgxpool.Pool` as concrete input; EventHandler takes `ProjectionWriter` interface instead
- [x] Service outputs are injected as interfaces: `worker.EventSubmitter` for Ingestion, `ProjectionWriter` for EventHandler
- [x] Each `Start()` returns `*RunningService` with `Shutdown` method
- [x] `cmd/platform/main.go` uses `Start()` calls instead of inline wiring
- [x] `cmd/platform/main.go` creates infrastructure (pools, producer, projections store) and passes them to services
- [x] `go test ./...` passes (unit tests)
- [x] `go test -tags=integration ./...` passes (22 integration tests)
- [x] E2E tests pass (3/3: full-flow, ingest-event, query-projection)
- [x] Existing `ProjectionWriter` interface in `eventhandler/repository.go` is used directly by EventHandler's `Start()` function (not duplicated)

**Implementation note:** The entry point function was renamed from `Service()` to `Start()` because each package already has a `Service` struct (the business logic type). `Start()` avoids the name collision while clearly communicating that it launches the service.

## Notes

### Why not a `Service` interface?

Each service has a different `Config` and different dependencies. A shared `Service` interface would require either generics or `interface{}` parameters — complexity for no benefit. The `RunningService` return type provides the uniform lifecycle contract that `main.go` needs.

### The ingestion LISTEN connection

The ingestion worker needs a dedicated `pgx.Conn` for PostgreSQL LISTEN (not from the pool — LISTEN holds a connection open indefinitely). This connection is created inside `Service()` from `cfg.DatabaseURL`. This is why `Config` includes `DatabaseURL` even though the pool is passed separately.

### Component testing enabled

After this refactoring, a component test can call:

```go
// Ingestion: real DB, mock output
svc, err := ingestion.Service(ctx, testCfg, testPool, mockSubmitter, testLogger)
// ... POST events via HTTP, assert mockSubmitter received them ...
svc.Shutdown(ctx)

// EventHandler: mock output, real Redpanda input
svc, err := eventhandler.Service(ctx, testCfg, mockWriter, testLogger)
// ... produce events to Redpanda, assert mockWriter received projection writes ...
svc.Shutdown(ctx)
```

Each service can be tested in isolation by mocking its output while keeping its input infrastructure real.

### Follow-up: Update design spec testing section

After implementation, update `platform-docs/design-spec.md` section 15:
- Add "Component tests" as a fourth tier in the 15.1 strategy table (between integration and E2E)
- Expand the "Service Component Tests" section to describe the `Service()` entry point pattern
- Document the "inject outputs, not inputs" principle as the interface boundary rule
- Show when to use component tests vs. integration vs. E2E

This should be done after Spec 011 is implemented, not before — the design spec should document what exists.

### Actions and TSDB deferred

The `actions/` and `tsdb/` directories are empty placeholders. Their `Service()` functions will be added when those services are implemented (Phase 2 and Phase 5 respectively).
