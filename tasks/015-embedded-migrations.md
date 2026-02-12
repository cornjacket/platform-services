# Spec 015: Embedded Migrations (Service-Owned, Auto-Applied on Startup)

**Type:** Spec
**Status:** Complete
**Created:** 2026-02-11

## Context

Migrations are currently applied externally via `make migrate-all`, which uses `docker compose exec postgres psql` to pipe SQL files into the database. This approach:

- Only works in skeleton mode (host binary + Postgres in Docker)
- Fails in fullstack mode (distroless container has no shell or filesystem)
- Cannot work in AWS ECS (no Docker Compose, no operator)

ADR-0010 established that each service owns its migration files. ADR-0016 extends this: each service embeds and auto-applies its own migrations on startup.

## Functionality

The platform binary runs all pending migrations before starting any services. Each service's migrations are embedded into the binary via `//go:embed` and applied using goose.

### Startup Sequence (updated)

```
main.go
  1. Load config
  2. Connect to databases (per-service, existing)
  3. Run migrations (NEW)
     ├── Ingestion migrations → ingestion DB (goose_ingestion table)
     ├── Event Handler migrations → eventhandler DB (goose_eventhandler table)
     └── (future: Query, TSDB, Actions → their DBs)
  4. Start services (existing)
  5. Wait for shutdown
```

If any migration fails, the binary exits with a non-zero code. No services start.

## Implementation

### Migration Library: goose

Chose `pressly/goose/v3` over `golang-migrate/migrate` because:
- Accepts existing `001_create_outbox.sql` naming as-is (no `.up.sql` rename needed)
- Simpler programmatic API
- Supports `io/fs` sources (compatible with `//go:embed`)

### Per-Service Version Tracking

**Critical design decision:** Each service uses a dedicated goose table name (`goose_ingestion`, `goose_eventhandler`) via `goose.SetTableName()`. Without this, services sharing the same database in dev would collide on version numbers — ingestion's version 2 would prevent event handler's version 1+2 from running.

In production (separate databases per ADR-0010), this doesn't matter, but the per-service table name is correct regardless.

### SQL Annotations

Goose requires `-- +goose Up` at the top of each SQL file. PL/pgSQL functions with `$$` dollar-quoting additionally need `-- +goose StatementBegin` / `-- +goose StatementEnd` to prevent goose from splitting on internal semicolons.

### database/sql Bridge

Goose requires `database/sql`, not `pgxpool`. The `RunMigrations` helper opens a temporary `sql.Open("pgx", databaseURL)` connection using the pgx stdlib driver adapter (`_ "github.com/jackc/pgx/v5/stdlib"`). This connection is separate from the pgxpool and closed after migration completes.

### Embedding Migrations

Each service package exposes an embedded FS:

```go
// internal/services/ingestion/migrations.go
package ingestion

import "embed"

//go:embed migrations/*.sql
var MigrationFS embed.FS
```

### Migration Runner

```go
// internal/shared/infra/postgres/migrate.go
func RunMigrations(databaseURL string, fsys fs.FS, subdir, tableName string) error
```

### Integration with main.go

```go
postgres.RunMigrations(cfg.DatabaseURLIngestion, ingestion.MigrationFS, "migrations", "goose_ingestion")
postgres.RunMigrations(cfg.DatabaseURLEventHandler, eventhandler.MigrationFS, "migrations", "goose_eventhandler")
```

### Makefile Changes

- `make migrate-all` retained as dev reset convenience; annotated as non-primary
- `make dev` simplified: `skeleton-up run` (removed `migrate-all` — binary self-migrates)

## Files Created/Modified

| File | Action | Purpose |
|------|--------|---------|
| `go.mod` | Modified | Added `pressly/goose/v3` dependency |
| `internal/services/ingestion/migrations.go` | Created | `//go:embed migrations/*.sql` |
| `internal/services/eventhandler/migrations.go` | Created | `//go:embed migrations/*.sql` |
| `internal/shared/infra/postgres/migrate.go` | Created | `RunMigrations()` helper with per-service table names |
| `cmd/platform/main.go` | Modified | Call `RunMigrations()` before `Start()` |
| `internal/services/ingestion/migrations/001_create_outbox.sql` | Modified | Added `-- +goose Up`, `StatementBegin/End` |
| `internal/services/ingestion/migrations/002_create_event_store.sql` | Modified | Added `-- +goose Up` |
| `internal/services/eventhandler/migrations/001_create_projections.sql` | Modified | Added `-- +goose Up` |
| `internal/services/eventhandler/migrations/002_create_dlq.sql` | Modified | Added `-- +goose Up` |
| `Makefile` | Modified | Added ADR-0016 comment, updated `dev` target |
| `platform-docs/decisions/0016-embedded-migration-on-startup.md` | Modified | Status → Accepted |
| `platform-docs/PROJECT.md` | Modified | Mark task complete |

## Acceptance Criteria

- [x] `pressly/goose/v3` added to `go.mod`
- [x] Each service package exposes `MigrationFS` via `//go:embed`
- [x] `RunMigrations()` helper created with per-service table names
- [x] SQL files annotated with `-- +goose Up` (and `StatementBegin/End` where needed)
- [x] `cmd/platform/main.go` runs migrations before starting services
- [x] Platform starts successfully against a fresh database (no prior `make migrate-all`)
- [x] Platform starts successfully against an already-migrated database (idempotent)
- [x] `make skeleton-up && go run ./cmd/platform` works without `make migrate-all`
- [x] Integration and component tests still pass
- [x] ADR-0016 status updated to Accepted
- [x] Update `platform-docs/PROJECT.md` to reflect task completion

## Notes

- No file renaming was needed (goose accepts `001_*.sql` naming)
- `testutil.MustRunMigrations()` is unaffected — it reads SQL from disk directly, not via goose
- Goose uses `goose_ingestion` and `goose_eventhandler` tables (not the default `goose_db_version`)
