# Spec 015: Embedded Migrations (Service-Owned, Auto-Applied on Startup)

**Type:** Spec
**Status:** Draft
**Created:** 2026-02-11

## Context

Migrations are currently applied externally via `make migrate-all`, which uses `docker compose exec postgres psql` to pipe SQL files into the database. This approach:

- Only works in skeleton mode (host binary + Postgres in Docker)
- Fails in fullstack mode (distroless container has no shell or filesystem)
- Cannot work in AWS ECS (no Docker Compose, no operator)

ADR-0010 established that each service owns its migration files. ADR-0016 (Proposed) extends this: each service embeds and auto-applies its own migrations on startup.

## Functionality

The platform binary runs all pending migrations before starting any services. Each service's migrations are embedded into the binary via `//go:embed` and applied using a migration library.

### Startup Sequence (updated)

```
main.go
  1. Load config
  2. Connect to databases (per-service, existing)
  3. Run migrations (NEW)
     ├── Ingestion migrations → ingestion DB
     ├── Event Handler migrations → eventhandler DB
     └── (future: Query, TSDB, Actions → their DBs)
  4. Start services (existing)
  5. Wait for shutdown
```

If any migration fails, the binary exits with a non-zero code. No services start.

## Design

### Migration Library

**`golang-migrate/migrate`** — widely used, supports `io/fs` source (for `//go:embed`), PostgreSQL driver, advisory lock for concurrent startup safety, tracks versions in `schema_migrations` table.

Alternative: `pressly/goose` — similar capabilities, Go-native migrations. Either works; `golang-migrate` is more common in the ecosystem.

### Embedding Migrations

Each service package that owns migrations adds an embed directive:

```go
// internal/services/ingestion/migrations.go
package ingestion

import "embed"

//go:embed migrations/*.sql
var MigrationFS embed.FS
```

```go
// internal/services/eventhandler/migrations.go
package eventhandler

import "embed"

//go:embed migrations/*.sql
var MigrationFS embed.FS
```

### Migration Runner

A shared helper in `internal/shared/infra/postgres/` (or `internal/shared/migrate/`):

```go
func RunMigrations(pool *pgxpool.Pool, fs embed.FS, subdir string) error
```

- Takes the service's database pool and embedded FS
- Extracts the migration source from the FS
- Applies pending migrations
- Returns error if any migration fails

### File Naming Convention

Migration files follow the existing convention (unchanged):

```
001_create_outbox.sql
002_create_event_store.sql
```

`golang-migrate` expects `{version}_{description}.up.sql` and optionally `{version}_{description}.down.sql`. The existing files need renaming:

| Current | New |
|---------|-----|
| `001_create_outbox.sql` | `001_create_outbox.up.sql` |
| `002_create_event_store.sql` | `002_create_event_store.up.sql` |
| `001_create_projections.sql` | `001_create_projections.up.sql` |
| `002_create_dlq.sql` | `002_create_dlq.up.sql` |

No `.down.sql` files — forward-only migrations per ADR-0016.

### Integration with `cmd/platform/main.go`

```go
// After connecting to databases, before starting services:
if err := postgres.RunMigrations(ingestionPool, ingestion.MigrationFS, "migrations"); err != nil {
    logger.Error("ingestion migration failed", "error", err)
    os.Exit(1)
}
if err := postgres.RunMigrations(eventHandlerPool, eventhandler.MigrationFS, "migrations"); err != nil {
    logger.Error("eventhandler migration failed", "error", err)
    os.Exit(1)
}
```

### Makefile Changes

- `make migrate-all` is retained but repurposed: used for local dev resets only (drop + re-create via `docker compose exec`)
- Add a comment clarifying that the binary handles migrations in production
- No new Makefile targets needed

### Test Impact

- **Integration/component tests:** Currently use `testutil.MustRunMigrations()` which reads SQL files from disk. This still works because tests run on the host (not in a container). No change needed.
- **Future consideration:** Tests could use the same `embed.FS` source, but this is not required for this spec.

## Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `go.mod` | Modify | Add `golang-migrate/migrate` dependency |
| `internal/services/ingestion/migrations.go` | Create | `//go:embed migrations/*.sql` |
| `internal/services/eventhandler/migrations.go` | Create | `//go:embed migrations/*.sql` |
| `internal/services/ingestion/migrations/*.sql` | Rename | Add `.up.sql` suffix |
| `internal/services/eventhandler/migrations/*.sql` | Rename | Add `.up.sql` suffix |
| `internal/shared/infra/postgres/migrate.go` | Create | `RunMigrations()` helper |
| `cmd/platform/main.go` | Modify | Call `RunMigrations()` before `Start()` |
| `Makefile` | Modify | Add clarifying comment to `migrate-all` |
| `platform-docs/design-spec.md` | Modify | Document embedded migration pattern |
| `platform-docs/PROJECT.md` | Modify | Mark task complete |

## Acceptance Criteria

- [ ] `golang-migrate/migrate` (or `goose`) added to `go.mod`
- [ ] Migration SQL files renamed with `.up.sql` suffix
- [ ] Each service package exposes `MigrationFS` via `//go:embed`
- [ ] `RunMigrations()` helper created and tested
- [ ] `cmd/platform/main.go` runs migrations before starting services
- [ ] Platform starts successfully against a fresh database (no prior `make migrate-all`)
- [ ] Platform starts successfully against an already-migrated database (idempotent)
- [ ] `make skeleton-up && go run ./cmd/platform` works without `make migrate-all`
- [ ] `make fullstack-up` works (container self-migrates)
- [ ] Integration and component tests still pass
- [ ] ADR-0016 status updated to Accepted
- [ ] Update `platform-docs/PROJECT.md` to reflect task completion

## Notes

- `golang-migrate` uses PostgreSQL advisory locks to prevent concurrent migration races. This is important for horizontal scaling (multiple ECS tasks starting simultaneously).
- The `schema_migrations` table is created automatically by the library. It tracks which version has been applied. This table is *not* owned by any service — it's owned by the migration framework.
- If `golang-migrate` file naming doesn't fit well (`.up.sql` suffix), `goose` uses simpler naming (`001_create_outbox.sql` works as-is). Evaluate during implementation.
