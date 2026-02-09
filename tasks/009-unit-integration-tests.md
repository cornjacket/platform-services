# Spec 009: Unit and Integration Tests

**Type:** Spec
**Status:** Draft
**Created:** 2026-02-09
**Updated:** 2026-02-09

## Context

The Phase 1 (Local Skeleton) checklist in `platform-docs/PROJECT.md` includes "Unit/Integration tests" as a deliverable. The codebase currently has **12 test functions across 5 files**, created as byproducts of other specs or chat-initiated work. There is no dedicated testing spec, no mock strategy, and no defined test baseline.

This spec serves as a **reconciliation point**: it documents the existing tests (attributing their origins), establishes the mock and test strategy, and defines the remaining test coverage needed for Phase 1.

## Current State

### Existing Tests (12 functions, 5 files)

| File | Tests | Origin |
|------|-------|--------|
| `internal/shared/domain/clock/clock_test.go` | 4 — `TestRealClock_Now`, `TestFixedClock_Now`, `TestReplayClock_Advance`, `TestPackageLevelClock` | Spec 008 |
| `internal/shared/domain/events/envelope_test.go` | 5 — `TestNewEnvelope`, `TestNewEnvelope_PayloadMarshaling`, `TestEnvelope_ParsePayload`, `TestNewEnvelope_InvalidPayload`, `TestNewEnvelope_DualTimestamps` | Spec 008 |
| `internal/shared/config/config_test.go` | 1 — `TestValidate` | Chat-initiated (no task doc) |
| `internal/services/ingestion/service_test.go` | 1 — `TestValidate` | Spec 007 |
| `internal/client/eventhandler/client_test.go` | 1 — `TestTopicFromEventType` | Spec 007 |

All existing tests pass. They cover domain objects (clock, envelope) and basic validation (config, ingestion service, topic routing) but do not cover HTTP handlers, service-layer flows, the outbox worker, or event handler dispatch.

## Functionality

Add comprehensive unit tests for all application-layer code written during Phase 1:
- HTTP handlers (ingestion, query)
- Service-layer business logic (ingestion flow, query pagination/validation)
- Outbox worker processor (3-step pipeline, retries, idempotency)
- Event handler registry and projection handlers
- Extend existing test files with additional scenarios

## Design

### Mock Strategy: Hand-Written Mocks

All interfaces in the codebase are small (1–3 methods). The project uses **zero external test libraries** — only the stdlib `testing` package. Hand-written mocks using the **function-field pattern** provide maximum flexibility without adding dependencies.

**Pattern:**
```go
type mockOutboxRepository struct {
    InsertFn func(ctx context.Context, event *events.Envelope) error
}

func (m *mockOutboxRepository) Insert(ctx context.Context, event *events.Envelope) error {
    return m.InsertFn(ctx, event)
}
```

This pattern allows each test to define exact behavior inline, without shared mock state or mock libraries.

### Mock Location: Co-located `testhelpers_test.go`

Each package that needs mocks gets a `testhelpers_test.go` file containing mock implementations. The `_test.go` suffix ensures:
- Mocks are **unexported** — scoped to the package's tests only
- Mocks are **not compiled** into the production binary
- Mocks live next to the code they test — no cross-package mock imports

### No Numeric Coverage Target

Instead of a percentage target (which incentivizes testing trivial code), acceptance criteria enumerate **specific functions and scenarios**. Phase 1 scope targets the code that has meaningful logic to verify.

### Scope: Phase 1 Only

The following are **explicitly deferred**:

| Deferred | Reason |
|----------|--------|
| Event Consumer (`consumer.go`) | Tightly coupled to Kafka/Redpanda client — needs interface extraction refactoring first |
| Infrastructure adapters (postgres packages) | Need real database; integration tests require Docker/testcontainers |
| Concurrency/race tests | Worker pool behavior requires careful goroutine orchestration; not a Phase 1 priority |
| E2E tests | Already covered by Spec 006 |

## Files to Create

### New Test Files (9)

| File | What It Tests |
|------|---------------|
| `internal/services/ingestion/handler_test.go` | `HandleIngest` (success, bad JSON, validation error, outbox error), `HandleHealth`, method not allowed |
| `internal/services/ingestion/testhelpers_test.go` | `mockOutboxRepository` (implements `OutboxRepository`) |
| `internal/services/query/handler_test.go` | `HandleGetProjection` (success, not found, invalid type, bad path), `HandleListProjections` (success, pagination params), `HandleHealth`, method not allowed |
| `internal/services/query/service_test.go` | `GetProjection` (success, invalid type, store error), `ListProjections` (success, pagination defaults, limit capping at 100, negative offset) |
| `internal/services/query/testhelpers_test.go` | `mockProjectionReader` (implements `ProjectionReader`) |
| `internal/services/ingestion/worker/processor_test.go` | `processEntry` (success, max retries exceeded, duplicate event in event store, submit error, delete error), `isDuplicateError` |
| `internal/services/ingestion/worker/testhelpers_test.go` | `mockOutboxReader`, `mockEventStoreWriter`, `mockEventSubmitter` |
| `internal/services/eventhandler/handlers_test.go` | `HandlerRegistry.Dispatch` (matched handler, no handler, error propagation), `SensorHandler.Handle` (success, store error), `UserHandler.Handle` (success, store error) |
| `internal/services/eventhandler/testhelpers_test.go` | `mockProjectionWriter` (implements `ProjectionWriter`), `mockEventHandler` (implements `EventHandler`) |

### Existing Test Files to Extend (3)

| File | What to Add |
|------|-------------|
| `internal/services/ingestion/service_test.go` | `TestIngest` — full `Ingest()` flow: success (envelope created, outbox called), validation failure, outbox error, optional `event_time` handling |
| `internal/client/eventhandler/client_test.go` | `TestSubmitEvent` — success (publisher called with correct topic), publish error propagation |
| `internal/shared/config/config_test.go` | `TestLoad` — `Load()` with env vars set, defaults when no env vars, validation failure on empty required fields |

## Interface Summary

For reference, these are the interfaces that mocks must implement:

```go
// ingestion package
type OutboxRepository interface {
    Insert(ctx context.Context, event *events.Envelope) error
}

// query package
type ProjectionReader interface {
    GetProjection(ctx context.Context, projType, aggregateID string) (*projections.Projection, error)
    ListProjections(ctx context.Context, projType string, limit, offset int) ([]projections.Projection, int, error)
}

// worker package
type OutboxReader interface {
    FetchPending(ctx context.Context, limit int) ([]OutboxEntry, error)
    Delete(ctx context.Context, outboxID string) error
    IncrementRetry(ctx context.Context, outboxID string) error
}
type EventStoreWriter interface {
    Insert(ctx context.Context, event *events.Envelope) error
}
type EventSubmitter interface {
    SubmitEvent(ctx context.Context, event *events.Envelope) error
}

// eventhandler package
type ProjectionWriter interface {
    WriteProjection(ctx context.Context, projType, aggregateID string, state []byte, event *events.Envelope) error
}
type EventHandler interface {
    Handle(ctx context.Context, event *events.Envelope) error
}
```

## Acceptance Criteria

- [ ] All 12 existing tests continue to pass
- [ ] Ingestion handler: tests cover success, bad JSON, validation error, outbox error, method not allowed, health
- [ ] Ingestion service: tests cover full `Ingest()` flow including optional `event_time`
- [ ] Query handler: tests cover get (success, not found, invalid type, bad path), list (success, pagination), health, method not allowed
- [ ] Query service: tests cover get/list with validation, pagination defaults, limit capping
- [ ] Worker processor: tests cover `processEntry` (success, max retries, duplicate, submit error, delete error) and `isDuplicateError`
- [ ] Event handler: tests cover `Dispatch` routing, `SensorHandler.Handle`, `UserHandler.Handle` (success and error paths)
- [ ] EventHandler client: tests cover `SubmitEvent` success and error propagation
- [ ] Config: tests cover `Load()` with env vars and defaults
- [ ] All mocks use hand-written function-field pattern in `testhelpers_test.go` files
- [ ] `go test ./...` passes with zero failures
- [ ] No external test dependencies added

## Notes

### Why "reconciliation point" instead of retroactive docs

The "every change needs a task doc" rule (Lesson 003) was adopted mid-project. Rather than creating retroactive task documents for the 12 existing tests (which would be historical fiction with fabricated timestamps), this spec acknowledges them in the Current State section with their actual origins. Going forward, all test changes will follow the standard task-doc-first workflow.

See: `ai-builder-lessons/lessons/004-retroactive-documentation-when-adopting-task-doc-rule.md`

### Test naming conventions

Following Go conventions:
- `TestFunctionName` — basic test
- `TestFunctionName_Scenario` — scenario-specific test
- Table-driven tests where there are 3+ related scenarios
- Subtests via `t.Run()` for table-driven cases

### Future considerations

- **testcontainers-go** — for integration tests against real PostgreSQL in Phase 2
- **Race detection** — `go test -race ./...` should pass but is not a formal criterion yet
- **Benchmark tests** — deferred until performance optimization phase
