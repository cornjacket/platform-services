# Spec 010: Integration Tests

**Type:** Spec
**Status:** Complete
**Created:** 2026-02-10
**Updated:** 2026-02-10

## Context

Phase 1 in `platform-docs/PROJECT.md` lists "Integration tests (real Postgres/Redpanda)" as a deliverable. Unit tests (Spec 009) are complete but mock all infrastructure. Integration tests verify the actual SQL queries, Kafka protocol behavior, and serialization round-trips that unit tests cannot cover.

Key things only integration tests can prove:
- JSONB serialization round-trips survive Postgres (Go → JSON → JSONB → JSON → Go)
- The `ON CONFLICT ... WHERE` clause in `WriteProjection` actually rejects older events
- The unique constraint on `event_store.event_id` returns pgconn error code 23505
- LISTEN/NOTIFY triggers fire on outbox insert
- franz-go produces messages that a consumer can read from Redpanda
- Partition key routing (same `aggregate_id` → same partition)

## Functionality

Add integration tests that exercise the concrete Postgres and Redpanda implementations against real infrastructure provided by the existing `docker-compose.yml`. Tests are gated behind `//go:build integration` so `go test ./...` continues to run only unit tests.

## Design

### Build Tag Isolation

Every integration test file uses `//go:build integration`. Running integration tests requires an explicit opt-in:

```sh
go test -tags=integration ./...
```

Regular `go test ./...` skips them entirely.

### Infrastructure: docker-compose (not testcontainers)

The existing `docker-compose.yml` already provides:
- **PostgreSQL** (TimescaleDB) on `localhost:5432` — user `cornjacket`, db `cornjacket`
- **Redpanda** on `localhost:9092` (Kafka-compatible)

No new dependencies or containers needed. Trade-off: developer must run `docker compose up -d` before integration tests. This is consistent with the existing dev workflow (`make dev` already starts docker).

### Test Isolation

**Postgres:** Each test truncates its tables before running. This is simpler than transaction rollback (which would require threading `pgx.Tx` through repos, reshaping the production API for testing). Table truncation tests the identical code path as production.

**Redpanda:** Each test uses a unique topic name (e.g., `test-<testname>-<timestamp>`) to avoid cross-test interference. Topics auto-create via `kgo.AllowAutoTopicCreation()`.

### Shared Test Helpers: `internal/testutil/`

New package with shared infrastructure helpers. All files gated with `//go:build integration`.

**`internal/testutil/postgres.go`:**
- `NewTestPool(t *testing.T) *pgxpool.Pool` — creates a pgxpool connection to the dev Postgres, calls `t.Cleanup()` to close
- `RunMigrations(t *testing.T, pool *pgxpool.Pool, migrationDir string)` — applies `.sql` files in order
- `TruncateTables(t *testing.T, pool *pgxpool.Pool, tables ...string)` — `TRUNCATE TABLE ... CASCADE`

**`internal/testutil/redpanda.go`:**
- `TestBrokers() []string` — returns `[]string{"localhost:9092"}`
- `TestTopicName(t *testing.T) string` — generates unique topic name from test name + timestamp

### Migrations in TestMain

Each integration test package runs `TestMain` to apply migrations before any tests execute, using the same `.sql` files the Makefile uses:

```go
func TestMain(m *testing.M) {
    pool := testutil.NewTestPool(...)
    testutil.RunMigrations(..., "../../services/ingestion/migrations")
    os.Exit(m.Run())
}
```

This ensures the schema is always current without requiring the developer to manually run `make migrate`.

### Connection Defaults

Tests use the same defaults as docker-compose:
```
INTEGRATION_DB_URL=postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable
INTEGRATION_REDPANDA_BROKERS=localhost:9092
```

Override via environment variables for CI or non-default setups.

## What to Test

### Postgres — OutboxRepo (`internal/shared/infra/postgres/outbox.go`)

| Test | What It Proves |
|------|----------------|
| `TestOutboxInsert` | `Insert` serializes envelope to JSONB correctly; row exists with expected columns |
| `TestOutboxFetchPending` | `FetchPending` returns rows ordered by `created_at ASC` with correct limit |
| `TestOutboxFetchPending_Empty` | Returns empty slice (not nil) when no rows |
| `TestOutboxDelete` | `Delete` removes the row |
| `TestOutboxDelete_MissingID` | Delete on non-existent ID does not error (logs warning) |
| `TestOutboxIncrementRetry` | `IncrementRetry` increments count from 0→1→2 |
| `TestOutboxInsertFetchRoundTrip` | Full insert→fetch cycle: JSON through Postgres JSONB and back, all envelope fields preserved |
| `TestOutboxNotifyTrigger` | LISTEN on `outbox_insert` channel receives notification after Insert (verifies trigger) |

### Postgres — EventStoreRepo (`internal/shared/infra/postgres/eventstore.go`)

| Test | What It Proves |
|------|----------------|
| `TestEventStoreInsert` | `Insert` stores all 7 columns (`event_id`, `event_type`, `aggregate_id`, `event_time`, `ingested_at`, `payload`, `metadata`) correctly |
| `TestEventStoreInsert_Duplicate` | Second insert with same `event_id` returns error; error is `pgconn.PgError` with code `23505` |
| `TestEventStoreInsertRoundTrip` | Insert→query: JSONB + TIMESTAMPTZ fidelity (timestamps survive round-trip without precision loss) |

### Postgres — PostgresStore (`internal/shared/projections/postgres.go`)

| Test | What It Proves |
|------|----------------|
| `TestWriteProjection_Insert` | First write creates new projection row |
| `TestWriteProjection_UpdateNewer` | Second write with newer `event_time` updates the row |
| `TestWriteProjection_SkipOlder` | Write with older `event_time` does NOT update (the `WHERE` clause in `ON CONFLICT`) — **highest-value integration test** |
| `TestWriteProjection_SameTimestamp_UUIDTiebreaker` | When timestamps match, larger UUID wins (the `AND projections.last_event_id < EXCLUDED.last_event_id` clause) |
| `TestGetProjection` | Retrieves correct projection by type + aggregate_id |
| `TestGetProjection_NotFound` | Returns error for non-existent projection |
| `TestListProjections` | Returns paginated results with correct total count |
| `TestListProjections_Empty` | Returns empty slice (not nil) and total=0 |

### Redpanda — Producer (`internal/shared/infra/redpanda/producer.go`)

| Test | What It Proves |
|------|----------------|
| `TestProducerPublish` | `Publish` produces a message consumable from the topic (produce, then consume with franz-go client to verify) |
| `TestProducerPartitionKey` | Same `aggregate_id` → same partition across multiple publishes |

### Redpanda — Consumer (`internal/services/eventhandler/consumer.go`)

| Test | What It Proves |
|------|----------------|
| `TestConsumerRoundTrip` | Produce message to topic → start consumer → handler receives deserialized envelope within timeout (5s). Verifies the full serialization→Redpanda→deserialization→dispatch path. |

## Files to Create/Modify

### New Files

| File | Purpose |
|------|---------|
| `internal/testutil/postgres.go` | Shared Postgres helpers (`NewTestPool`, `RunMigrations`, `TruncateTables`). Build tag: `//go:build integration` |
| `internal/testutil/redpanda.go` | Shared Redpanda helpers (`TestBrokers`, `TestTopicName`). Build tag: `//go:build integration` |
| `internal/shared/infra/postgres/outbox_integration_test.go` | OutboxRepo tests (~8 tests). Build tag: `//go:build integration` |
| `internal/shared/infra/postgres/eventstore_integration_test.go` | EventStoreRepo tests (~3 tests). Build tag: `//go:build integration` |
| `internal/shared/projections/postgres_integration_test.go` | PostgresStore tests (~8 tests). Build tag: `//go:build integration` |
| `internal/shared/infra/redpanda/producer_integration_test.go` | Producer tests (~2 tests). Build tag: `//go:build integration` |
| `internal/services/eventhandler/consumer_integration_test.go` | Consumer test (~1 test). Build tag: `//go:build integration` |

### Modified Files

| File | Change |
|------|--------|
| `Makefile` | Add `test-integration` target (`go test -tags=integration -v ./...`) and `test-all` target (runs both unit and integration) |
| `platform-docs/PROJECT.md` | Update wording from "via testcontainers" to "via docker-compose" |

### No Existing Test Files Modified

The `//go:build integration` tag makes integration test files invisible to `go test ./...`. No production code changes needed. No existing unit test files need modification.

## Acceptance Criteria

- [x] `internal/testutil/` package provides `NewTestPool`, `MustNewTestPool`, `RunMigrations`, `MustRunMigrations`, `MustDropAllTables`, `TruncateTables`, `TestBrokers`, `TestTopicName`
- [x] All testutil files have `//go:build integration` tag
- [x] OutboxRepo: 8 integration tests covering insert, fetch, delete, retry, round-trip, and NOTIFY trigger
- [x] EventStoreRepo: 3 integration tests covering insert, duplicate constraint (pgconn 23505), and round-trip
- [x] PostgresStore: 8 integration tests covering write (insert/update/skip-older/UUID-tiebreaker), get, get-not-found, list, list-empty
- [x] Producer: 2 integration tests covering publish verification and partition key routing
- [x] Consumer: 1 integration test covering produce→consume→dispatch round-trip
- [x] Each `TestMain` drops all tables then runs migrations (`MustDropAllTables` → `MustRunMigrations`), owning the full schema lifecycle
- [x] Each test truncates tables as its first action (Postgres) or uses unique topic names (Redpanda) for row-level isolation
- [x] `go test ./...` continues to pass (integration tests invisible without tag)
- [x] `go test -tags=integration ./...` runs all 22 integration tests (requires `docker compose up -d`)
- [x] Makefile has `test-integration` and `test-all` targets
- [x] PROJECT.md updated: "via testcontainers" → "via docker-compose"
- [x] Migration 002 rewritten with final schema (`event_time` + `ingested_at`); migration 003 (non-idempotent ALTER) deleted

## Notes

### Why docker-compose over testcontainers

The plan originally specified testcontainers (PROJECT.md wording). Docker-compose was chosen instead because:

1. **Already exists** — `docker-compose.yml` is checked in with the exact Postgres (TimescaleDB) and Redpanda versions the platform uses. No version drift.
2. **No new dependency** — testcontainers-go requires an additional Go module and Docker socket access with specific permissions.
3. **Faster iteration** — containers stay running between test runs. testcontainers spins up fresh containers per `go test` invocation (~5-10s startup per container).
4. **Matches dev workflow** — `make dev` already starts docker-compose. Integration tests use the same infrastructure developers already have running.

Trade-off: tests are not fully self-contained. Developer must run `docker compose up -d` first. This is acceptable for Phase 1 dev-machine testing. If CI needs self-contained tests later, testcontainers can be layered on without changing test logic (only the helper functions in `internal/testutil/` would change).

### Why not transaction rollback for isolation

Transaction rollback (begin tx → run test → rollback) is cleaner in theory but requires threading `pgx.Tx` through every repo method. The current repos accept `*pgxpool.Pool` and use `pool.Exec()`/`pool.Query()` directly. Refactoring to accept a `pgx.Tx` (or a union interface) would change production code's API purely for testing ergonomics. Table truncation is cruder but tests the identical code path as production.

### Test count summary

| Component | Tests |
|-----------|-------|
| OutboxRepo | 8 |
| EventStoreRepo | 3 |
| PostgresStore | 8 |
| Producer | 2 |
| Consumer | 1 |
| **Total** | **22** |

### No interfaces needed

Unlike unit tests (Spec 009) which required defining mock implementations, integration tests exercise the concrete types directly (`OutboxRepo`, `EventStoreRepo`, `PostgresStore`, `Producer`, `Consumer`). The only new code is the `testutil` helpers — no production code refactoring.

---

## Amendment: Drop-and-recreate schema in TestMain + consolidate migrations

**Date:** 2026-02-10
**Reason:** Post-implementation review revealed two issues: `MustRunMigrations` was a no-op with error suppression, and migration 003 (an ALTER) was unnecessary pre-production.

### Problem

The original design had `TestMain` call `MustRunMigrations` to ensure the schema existed before tests ran. In practice, this was a no-op — the database already had the schema from `make migrate`. Worse, migration 003 (`ALTER TABLE RENAME COLUMN timestamp TO event_time`) isn't idempotent, so `MustRunMigrations` had to swallow errors and log "skipping — likely already applied." This created a false safety net: it looked like it was doing something useful but was actually just producing log noise.

### Root cause (two issues)

**1. `TestMain` didn't own the schema lifecycle.** It assumed migrations would be harmless to re-run. Some DDL is idempotent (`CREATE TABLE IF NOT EXISTS`), but `ALTER TABLE RENAME COLUMN` is not. Suppressing errors masked the real issue.

**2. Migration 003 is an ALTER that only makes sense with a live deployment.** ALTER migrations exist to transform existing data in-place — renaming a column while preserving rows. There is no live deployment. There is no data to preserve. The column should have been named `event_time` from the start.

### Fix (two parts)

**Part 1: Consolidate migrations (pre-production cleanup)**

Rewrite migration 002 to define the final schema with `event_time` and `ingested_at` from the start. Delete migration 003 entirely. This makes all remaining migrations purely `CREATE TABLE/INDEX IF NOT EXISTS` — naturally idempotent.

This is a pre-production-only action. Once a production deployment exists, ALTER migrations are the correct approach and must not be rewritten.

**Part 2: Drop-and-recreate in TestMain (future-proof pattern)**

Even though Part 1 makes current migrations idempotent, `TestMain` should still use drop-then-migrate. Future post-production migrations will contain ALTERs that aren't idempotent. Establishing the pattern now means those future migrations work automatically in the test suite without any changes to test infrastructure.

`TestMain` flow:

```
MustDropAllTables → MustRunMigrations → m.Run()
```

`MustDropAllTables` implementation:

```sql
DO $$ DECLARE
    r RECORD;
BEGIN
    FOR r IN (SELECT tablename FROM pg_tables WHERE schemaname = 'public') LOOP
        EXECUTE 'DROP TABLE IF EXISTS ' || quote_ident(r.tablename) || ' CASCADE';
    END LOOP;
END $$
```

`MustRunMigrations` reverts to the strict version — `log.Fatal` on any error, no skipping. Errors are always real because the schema is always blank.

### What changes

| File | Change |
|------|--------|
| `internal/services/ingestion/migrations/002_create_event_store.sql` | Rewrite with `event_time` and `ingested_at` columns and correct indexes (final schema) |
| `internal/services/ingestion/migrations/003_dual_timestamps.sql` | Delete |
| `internal/testutil/postgres.go` | Add `MustDropAllTables`. Revert `MustRunMigrations` and `RunMigrations` to fatal on errors (remove skip logic). |
| `internal/shared/infra/postgres/outbox_integration_test.go` | `TestMain` calls `MustDropAllTables` before `MustRunMigrations` |
| `internal/shared/projections/postgres_integration_test.go` | Same |

### Why this is better

- **Migrations are exercised on every test run** — a broken migration is caught immediately, not just when someone sets up a fresh database
- **No idempotency workaround** — migrations run against a clean schema, so every migration's assumptions hold
- **`TestMain` owns the full schema lifecycle** — no dependency on `make migrate` having been run first
- **Future-proof** — post-production ALTER migrations will work in tests without any infrastructure changes
- **`TruncateTables` per test still handles row-level isolation** — schema is set up once in `TestMain`, rows are cleaned between individual tests

### Trade-off

If you're manually inserting test data via `psql` while developing, integration tests will nuke it. This is expected — integration tests own the database state.
