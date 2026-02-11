# Spec 012: Component Tests

**Type:** Spec
**Status:** Complete
**Created:** 2026-02-10
**Updated:** 2026-02-10

## Context

Unit tests (Spec 009) mock all dependencies. Integration tests (Spec 010) exercise individual infrastructure adapters against real Postgres/Redpanda. Neither tests a complete service end-to-end within its boundary.

Spec 011 introduced `Start()` entry points that encapsulate each service's wiring. These are the natural boundary for component tests: start a real service, feed it real input, and capture its output via a mock.

Component tests fill the gap between integration and e2e tests:

| Test Type | What It Tests | Infra | Mocks |
|-----------|--------------|-------|-------|
| Unit | Individual functions | None | All dependencies |
| Integration | Single adapter (repo, producer) | Real DB or Redpanda | None |
| **Component** | **Full service pipeline** | **Real DB + Redpanda** | **Service output only** |
| E2E | All services together | Everything | None |

## Functionality

Add component tests that exercise each service through its `Start()` entry point. Tests use real infrastructure for inputs (Postgres, Redpanda) and channel-based mocks for outputs (EventSubmitter, ProjectionWriter).

## Design

### Build Tag

All component test files use `//go:build component`. This keeps them separate from both unit tests (`go test ./...`) and integration tests (`go test -tags=integration ./...`).

```sh
go test -tags=component ./internal/services/...
```

Component tests require `docker compose up -d` (same as integration tests).

### Test Files

| File | Service | Tests |
|------|---------|-------|
| `internal/services/ingestion/ingestion_test.go` | Ingestion | HTTP ingest → outbox → worker → mock EventSubmitter |
| `internal/services/eventhandler/eventhandler_test.go` | EventHandler | Produce to Redpanda → consumer → mock ProjectionWriter |
| `internal/services/query/query_test.go` | Query | Seed projections → HTTP GET → verify response |

### Channel-Based Mock Pattern

Component tests need to assert on asynchronous service outputs. The mock's method writes to a channel; the test blocks on it with a timeout.

```go
type channelSubmitter struct {
    calls chan *events.Envelope
}

func (m *channelSubmitter) SubmitEvent(ctx context.Context, event *events.Envelope) error {
    m.calls <- event
    return nil
}

// In test:
mock := &channelSubmitter{calls: make(chan *events.Envelope, 10)}
svc, _ := ingestion.Start(ctx, cfg, pool, mock, logger)

// POST event via HTTP
httpPost(...)

// Assert — wakes up the instant the worker calls SubmitEvent()
select {
case event := <-mock.calls:
    assert.Equal(t, "sensor.reading", event.EventType)
case <-time.After(5 * time.Second):
    t.Fatal("timed out waiting for event submission")
}
```

This pattern replaces the polling loop used in the existing `TestConsumerRoundTrip` integration test (mutex + 50ms sleep). Benefits:

- **Instant wake-up** — no polling interval means faster tests
- **No mutex** — channel synchronization replaces manual locking
- **Clear timeout** — single `select` with deadline, not a loop
- **Buffered channel** — `make(chan ..., 10)` prevents mock from blocking the service if multiple events are processed before the test reads

### Test Scenarios

#### Ingestion Component Test

**Setup:** Real Postgres pool, channel-based `EventSubmitter` mock, `Start()` on a test port.

| Test | What It Proves |
|------|----------------|
| `TestIngestion_IngestToSubmit` | POST event → HTTP 202 → outbox → worker processes → mock receives event with correct fields |
| `TestIngestion_InvalidPayload` | POST invalid JSON → HTTP 400, mock receives nothing |
| `TestIngestion_EventStoreWrite` | After submission, query event_store to verify the event was persisted |

The first test is the highest-value component test in the system. It exercises: HTTP parsing → validation → outbox insert → NOTIFY trigger → worker fetch → event store write → event submission. No other test type can cover this full path.

#### EventHandler Component Test

**Setup:** Channel-based `ProjectionWriter` mock, `Start()` with real Redpanda brokers and unique topic/group.

| Test | What It Proves |
|------|----------------|
| `TestEventHandler_SensorEvent` | Produce sensor event to Redpanda → consumer deserializes → mock receives `WriteProjection("sensor_state", ...)` |
| `TestEventHandler_UserEvent` | Produce user event → mock receives `WriteProjection("user_session", ...)` |
| `TestEventHandler_UnknownEventType` | Produce unknown type → no call to mock (handler skips gracefully) |

#### Query Component Test

**Setup:** Real Postgres pool, seed projections via `projections.NewPostgresStore(testPool, logger).WriteProjection(...)`, `Start()` on a test port. This uses the same write path as EventHandler in production — no raw SQL.

| Test | What It Proves |
|------|----------------|
| `TestQuery_GetProjection` | Seed projection row → HTTP GET → correct JSON response |
| `TestQuery_GetProjection_NotFound` | HTTP GET for non-existent → 404 |
| `TestQuery_ListProjections` | Seed multiple rows → HTTP GET with pagination → correct count, correct page |

Query is the simplest — no async behavior, no mock needed. The "output" is the HTTP response, which the test reads directly.

### Test Infrastructure

#### TestMain

Ingestion and Query component tests need `TestMain` for schema lifecycle (same pattern as integration tests):

```go
func TestMain(m *testing.M) {
    pool := testutil.MustNewTestPool()
    testutil.MustDropAllTables(pool)
    testutil.MustRunMigrations(pool, "path/to/migrations")
    testPool = pool
    defer pool.Close()
    os.Exit(m.Run())
}
```

EventHandler does not need `TestMain` — it has no database dependency (projections are mocked).

#### HTTP Test Helpers

Ingestion and Query tests need to make HTTP requests. A small helper in testutil or inline:

```go
func httpPost(t *testing.T, url string, body any) *http.Response {
    t.Helper()
    jsonBody, err := json.Marshal(body)
    require.NoError(t, err)
    resp, err := http.Post(url, "application/json", bytes.NewReader(jsonBody))
    require.NoError(t, err)
    return resp
}
```

#### Test Ports

Component tests use dedicated ports to avoid conflicts with a running platform instance:

| Service | Test Port |
|---------|-----------|
| Ingestion | 18080 |
| Query | 18081 |

These are arbitrary and only used during test runs. No config change needed — the test passes them via the `Config` struct.

#### Ingestion LISTEN Connection

The ingestion `Start()` creates a dedicated `pgx.Conn` for LISTEN. This needs `cfg.DatabaseURL` in the config. Component tests use the same test database URL as integration tests:

```go
cfg := ingestion.Config{
    Port:        18080,
    DatabaseURL: "postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable",
    WorkerCount: 1,   // single worker for predictable test behavior
    BatchSize:   10,
    MaxRetries:  3,
    PollInterval: 100 * time.Millisecond, // fast polling for test speed
}
```

## Files to Create/Modify

### New Files

| File | Purpose |
|------|---------|
| `internal/services/ingestion/ingestion_test.go` | Ingestion component tests (~3 tests). Build tag: `//go:build component` |
| `internal/services/eventhandler/eventhandler_test.go` | EventHandler component tests (~3 tests). Build tag: `//go:build component` |
| `internal/services/query/query_test.go` | Query component tests (~3 tests). Build tag: `//go:build component` |

### Modified Files

| File | Change |
|------|--------|
| `Makefile` | Add `test-component` target |

### No Changes Required

| File | Why |
|------|-----|
| `internal/services/*/ingestion.go`, `eventhandler.go`, `query.go` | `Start()` functions unchanged |
| `internal/testutil/*` | Existing helpers sufficient (pool, migrations, truncation, brokers, topics) |
| Existing unit tests | `//go:build component` tag makes component tests invisible to `go test ./...` |
| Existing integration tests | Different build tag |

## Acceptance Criteria

- [ ] `internal/services/ingestion/ingestion_test.go` with `//go:build component` — 3 tests using channel-based mock
- [ ] `internal/services/eventhandler/eventhandler_test.go` with `//go:build component` — 3 tests using channel-based mock
- [ ] `internal/services/query/query_test.go` with `//go:build component` — 3 tests using real Postgres
- [ ] Channel-based mock pattern used for async assertions (no polling loops)
- [ ] `TestIngestion_IngestToSubmit` proves the full HTTP → outbox → worker → submit path
- [ ] Each test cleans up: truncates tables, shuts down service
- [ ] `go test ./...` still passes (component tests invisible without tag)
- [ ] `go test -tags=component ./internal/services/...` runs component tests
- [ ] Makefile has `test-component` target
- [ ] `test-all` target updated to include component tests

## Notes

### Why a separate build tag instead of `integration`?

Component tests and integration tests have different concerns:

- **Integration tests** verify individual adapters (does `OutboxRepo.Insert` write the correct JSONB?). They're fast, focused, and test one thing.
- **Component tests** verify a full service pipeline (does an HTTP POST eventually trigger an event submission?). They're slower, start HTTP servers and workers, and test the wiring.

A developer debugging an outbox query wants to run integration tests for `postgres/`. They don't want to wait for the ingestion HTTP server to spin up. Separate tags allow targeted runs.

### Test count summary

| Component | Tests |
|-----------|-------|
| Ingestion | 3 |
| EventHandler | 3 |
| Query | 3 |
| **Total** | **9** |

### Why WorkerCount=1 in ingestion tests?

Multiple workers make test assertions non-deterministic — you can't predict which worker processes the entry. A single worker ensures the channel receives events in the order they were submitted, making assertions straightforward.
