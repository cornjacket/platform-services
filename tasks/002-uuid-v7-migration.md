# Task 002: UUID v7 Migration

**Status:** Complete
**Created:** 2026-02-04
**Updated:** 2026-02-04

## Context

ADR-0013 standardizes on UUID v7 across the platform. This task implements that decision.

See: [ADR-0013 UUID v7 Standardization](../../platform-docs/decisions/0013-uuid-v7-standardization.md)

## Functionality

Migrate all UUID generation from v4 to v7:
- Application-generated UUIDs (Go)
- Database-generated UUIDs (PostgreSQL)

## Design

### PostgreSQL 18 Upgrade

Upgrade to `timescale/timescaledb:2.23.0-pg18` which has native `uuidv7()` function.

We verified that `pg_uuidv7` extension was **not available** in PG15, but PG18 has native support — no extension needed.

### Docker Compose Changes

```yaml
# Before
image: timescale/timescaledb:2.11.2-pg15

# After
image: timescale/timescaledb:2.23.0-pg18
```

### Go Changes

Replace UUID library and generation:

```go
// Before (envelope.go)
import "github.com/google/uuid"
EventID: uuid.New()  // v4

// After
import "github.com/gofrs/uuid/v5"
EventID: uuid.Must(uuid.NewV7())  // v7
```

### PostgreSQL Migration Changes

Replace `uuid-ossp` extension with native `uuidv7()`:

```sql
-- Before
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE TABLE outbox (
    outbox_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    ...
);

-- After (no extension needed in PG18)
CREATE TABLE outbox (
    outbox_id UUID PRIMARY KEY DEFAULT uuidv7(),
    ...
);
```

## Files to Modify

### Docker
- `docker-compose.yml` — Upgrade to `timescale/timescaledb:2.23.0-pg18`

### Go
- `go.mod` — Replace `github.com/google/uuid` with `github.com/gofrs/uuid/v5`
- `internal/shared/domain/events/envelope.go` — Use `uuid.NewV7()`

### Migrations
- `internal/services/ingestion/migrations/001_create_outbox.sql` — Replace `uuid_generate_v4()` with `uuidv7()`, remove extension
- `internal/services/eventhandler/migrations/001_create_projections.sql` — Replace `uuid_generate_v4()` with `uuidv7()`, remove extension
- `internal/services/eventhandler/migrations/002_create_dlq.sql` — Replace `uuid_generate_v4()` with `uuidv7()`

## Acceptance Criteria

- [x] docker-compose.yml uses `timescale/timescaledb:2.23.0-pg18`
- [x] Go code generates UUID v7 for EventID
- [x] All migrations use `uuidv7()` (no extension needed)
- [x] Existing tests pass (no test failures)
- [x] New events have time-ordered UUIDs (verified: ORDER BY uuid sorts chronologically)

## Notes

- `event_store.event_id` has no DEFAULT — it receives the application-generated UUID
- This is a **breaking change** for any existing data (v4 and v7 UUIDs will coexist)
- Since we're in Phase 1 with no production data, this is safe to do now
- After upgrading, run `docker compose down -v && docker compose up -d` to reset with new image
