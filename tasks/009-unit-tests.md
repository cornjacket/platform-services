# Spec 009: Unit Tests

**Type:** Spec
**Status:** Complete
**Created:** 2026-02-09
**Updated:** 2026-02-09

## Context

The Phase 1 (Local Skeleton) checklist in `platform-docs/PROJECT.md` includes "Unit tests" as a deliverable. The codebase currently has **12 test functions across 5 files**, created as byproducts of other specs or chat-initiated work. There is no dedicated testing spec, no mock strategy, and no defined test baseline.

This spec covers **unit tests only** — all dependencies are mocked. Integration tests (real Postgres, real Redpanda via testcontainers) are a separate Phase 1 deliverable.

This spec serves as a **reconciliation point**: it documents the existing tests (attributing their origins), establishes the mock and test strategy, and defines the remaining unit test coverage needed for Phase 1.

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

All interfaces in the codebase are small (1–3 methods). Hand-written mocks using the **function-field pattern** provide maximum flexibility without adding dependencies.

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

### Scope: Unit Tests Only

This spec covers mocked unit tests. The following are **out of scope**:

| Out of Scope | Where It Lives |
|--------------|----------------|
| Integration tests (real Postgres, Redpanda) | Separate Phase 1 deliverable in PROJECT.md |
| Event Consumer (`consumer.go`) | Needs interface extraction refactoring first |
| Concurrency/race tests | Worker pool goroutine behavior — separate effort |
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
- [ ] Test dependencies evaluated case-by-case; `testify/assert` adopted for assertion noise reduction

## Notes

### Why "reconciliation point" instead of retroactive docs

The "every change needs a task doc" rule (Lesson 003) was adopted mid-project. Rather than creating retroactive task documents for the 12 existing tests (which would be historical fiction with fabricated timestamps), this spec acknowledges them in the Current State section with their actual origins. Going forward, all test changes will follow the standard task-doc-first workflow.

See: `ai-builder-lessons/lessons/004-retroactive-documentation-when-adopting-task-doc-rule.md`

### Why hand-written mocks over mock libraries

Mock libraries like `testify/mock` and `gomock` are popular in Go, but they solve a problem this codebase doesn't have.

**The core tradeoff is compile-time vs. runtime safety.** testify/mock uses string-based method dispatch (`mockRepo.On("Insert", mock.Anything).Return(nil)`). A typo in the method name compiles fine and fails at runtime with an opaque error. The function-field pattern catches the same mistake at compile time — there's no `InsertFn` field to misspell without the compiler objecting.

**Go's interface philosophy makes mock libraries unnecessary for small interfaces.** The stdlib averages 1-2 methods per interface (`io.Reader`, `http.Handler`). This codebase follows the same pattern — every interface is 1-3 methods. A hand-written mock for a 1-method interface is 5 lines. A mock library adds a dependency, a learning curve, and a runtime failure mode to avoid writing those 5 lines.

**Mock libraries originated in Java/C# where interfaces are large** (10-20 methods). Mocking a 15-method interface by hand is genuinely painful. But Go's "accept interfaces, return structs" principle means you define narrow interfaces at the consumer, not broad interfaces at the producer. Importing a solution for large interfaces into a codebase with small interfaces is solving a problem that doesn't exist.

**`mock.Anything` undermines static typing.** Once tests use `mock.Anything` to avoid specifying argument types, they've recreated dynamic typing inside a statically-typed language. The function-field pattern gets typed arguments for free because the function signature matches the interface method.

**Re-evaluate if interfaces grow.** If Phase 2 introduces interfaces with 5+ methods (e.g., wrapping a complex third-party SDK), gomock's code generation approach (which preserves compile-time safety unlike testify/mock) would be worth reconsidering. For now, hand-written mocks are the simpler, safer, and more idiomatic choice.

### Assertion library: `testify/assert` (not `testify/mock`)

Test dependencies are evaluated case-by-case rather than blanket-prohibited. `testify/assert` is adopted because the noise reduction is substantial — stdlib assertions require 3 lines (if/comparison/t.Errorf with format string) where `assert.Equal` takes one. With 50+ tests, this is real readability overhead.

This is distinct from `testify/mock`, which is *not* adopted. `assert` is pure syntactic sugar — no runtime dispatch, no string-based method names, no type safety tradeoffs. `mock` has fundamental compile-time safety problems (see "Why hand-written mocks" below). They are separate packages with separate tradeoffs.

### Test naming conventions

Following Go conventions:
- `TestFunctionName` — basic test
- `TestFunctionName_Scenario` — scenario-specific test
- Table-driven tests where there are 3+ related scenarios
- Subtests via `t.Run()` for table-driven cases

### Future considerations

- **testcontainers-go** — for integration tests against real PostgreSQL (separate Phase 1 spec)
- **Race detection** — `go test -race ./...` should pass but is not a formal criterion yet
- **Benchmark tests** — deferred until performance optimization phase
