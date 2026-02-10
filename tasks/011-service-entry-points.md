# Spec 011: Service Entry Points

**Type:** Spec
**Status:** Draft
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
- Takes a `*pgxpool.Pool` as a concrete dependency (the DB resource)
- Takes other dependencies as interfaces (for testability)
- Takes a logger
- Owns all internal wiring: creating repos, services, handlers, routes, servers
- Returns a `Shutdown()` function or similar mechanism for graceful teardown

## Design

### New Files

Each service gets a new entry point file:

| File | Function |
|------|----------|
| `internal/services/ingestion/ingestion.go` | `func Service(cfg Config, pool *pgxpool.Pool, submitter EventSubmitter, logger *slog.Logger) (*RunningService, error)` |
| `internal/services/eventhandler/eventhandler.go` | `func Service(cfg Config, pool *pgxpool.Pool, logger *slog.Logger) (*RunningService, error)` |
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

type RunningService struct {
    Shutdown func(ctx context.Context) error
}

func Service(ctx context.Context, cfg Config, pool *pgxpool.Pool, logger *slog.Logger) (*RunningService, error)
```

Internally, `Service()` creates:
- `projections.PostgresStore` from `pool`
- `HandlerRegistry` with `SensorHandler` and `UserHandler`
- `Consumer` with config
- Starts consumer in goroutine
- Returns `RunningService` with a `Shutdown` that closes the consumer

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
| Ingestion | `*pgxpool.Pool` | `EventSubmitter` | Redpanda producer is external; mockable for component tests |
| EventHandler | `*pgxpool.Pool` | (none) | Consumer is internal to the service; broker config is in `Config` |
| Query | `*pgxpool.Pool` | (none) | Read-only; projections store is created from pool internally |

The DB pool is concrete because:
- It's a resource, not a behavior — you don't mock a connection pool
- Services create their own repos from the pool (the repos are the abstraction layer)
- Per ADR-0010, each service gets its own pool in production

### RunningService Pattern

All three `Service()` functions return `*RunningService` with a `Shutdown` method. This gives `main.go` a uniform way to manage lifecycle:

```go
ingestion, err := ingestion.Service(ctx, ingestionCfg, ingestionPool, submitter, logger)
// ...
eventHandler, err := eventhandler.Service(ctx, ehCfg, ehPool, logger)
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
3. Create DB pools (one per service)
4. Create shared external resources (Redpanda producer → `eventhandler.Client`)
5. Call each `Service()` function
6. Wait for OS signal
7. Call `Shutdown()` on each service

All service-internal wiring (repos, handlers, routes, servers) moves into the respective `Service()` functions. `main.go` should drop from ~238 lines to ~80-100 lines.

### Existing Interfaces Stay Unchanged

The interfaces defined in each service's `repository.go` (`OutboxRepository`, `OutboxReader`, `EventStoreWriter`, `ProjectionWriter`, `ProjectionReader`, `EventHandler`, `EventConsumer`) are not modified. They continue to serve as the internal dependency boundaries within each service. The new `Service()` functions create the concrete implementations and wire them together.

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

- [ ] `internal/services/ingestion/ingestion.go` defines `Config`, `RunningService`, and `func Service()`
- [ ] `internal/services/eventhandler/eventhandler.go` defines `Config`, `RunningService`, and `func Service()`
- [ ] `internal/services/query/query.go` defines `Config`, `RunningService`, and `func Service()`
- [ ] Each `Service()` takes `*pgxpool.Pool` as concrete input, non-DB dependencies as interfaces
- [ ] Each `Service()` returns `*RunningService` with `Shutdown` method
- [ ] `cmd/platform/main.go` uses `Service()` calls instead of inline wiring
- [ ] `cmd/platform/main.go` has no direct imports of infrastructure packages (`postgres`, `redpanda`, `projections`) except for pool creation and the Redpanda producer
- [ ] `go test ./...` passes (unit tests)
- [ ] `go test -tags=integration ./...` passes (integration tests)
- [ ] `make dev` starts the platform successfully
- [ ] Existing `EventSubmitter` interface in `worker/repository.go` is reused (not duplicated) by the ingestion `Service()` function

## Notes

### Why not a `Service` interface?

Each service has a different `Config` and different dependencies. A shared `Service` interface would require either generics or `interface{}` parameters — complexity for no benefit. The `RunningService` return type provides the uniform lifecycle contract that `main.go` needs.

### The ingestion LISTEN connection

The ingestion worker needs a dedicated `pgx.Conn` for PostgreSQL LISTEN (not from the pool — LISTEN holds a connection open indefinitely). This connection is created inside `Service()` from `cfg.DatabaseURL`. This is why `Config` includes `DatabaseURL` even though the pool is passed separately.

### Component testing enabled

After this refactoring, a component test can call:

```go
svc, err := ingestion.Service(ctx, testCfg, testPool, mockSubmitter, testLogger)
// ... exercise HTTP endpoints ...
svc.Shutdown(ctx)
```

This starts a real HTTP server with a real database but a mock event submitter — the definition of a component test.

### Actions and TSDB deferred

The `actions/` and `tsdb/` directories are empty placeholders. Their `Service()` functions will be added when those services are implemented (Phase 2 and Phase 5 respectively).
