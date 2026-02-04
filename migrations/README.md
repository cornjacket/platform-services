# Database Migrations

## What Are Migrations?

"Migration" means a **versioned change to the database schema**. The term comes from "migrating" the database from one state to another.

Migrations are:
- **Numbered sequentially** (001, 002, 003...) so they run in order
- **Incremental** - each builds on the previous state
- **Reproducible** - run the same migrations, get the same schema

The first migrations create tables, but future migrations might:
- Add or remove columns (`ALTER TABLE`)
- Add indexes for performance
- Rename fields
- Transform existing data

## Migration Files

| File | Creates | Purpose |
|------|---------|---------|
| `001_create_outbox.sql` | `outbox` table | Temporary holding area for the outbox-first write pattern. Events land here first, then get processed to event_store + Redpanda. Includes NOTIFY trigger for real-time processing. |
| `002_create_event_store.sql` | `event_store` table | Append-only log of all events (CQRS write side). The permanent source of truth. |
| `003_create_projections.sql` | `projections` table | Materialized views optimized for queries (CQRS read side). Updated by the Event Handler. |
| `004_create_dlq.sql` | `dlq` table | Dead letter queue for events that failed processing. Enables debugging and replay. |

## Running Migrations

```bash
# Run all migrations (via Docker)
make migrate

# Run all migrations (local psql)
make migrate-local
```

## Adding New Migrations

1. Create a new file with the next number: `005_description.sql`
2. Write idempotent SQL when possible (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`)
3. Run `make migrate` to apply

## Schema Reference

See [design-spec.md](../../platform-docs/design-spec.md) Section 4 for detailed schema documentation.
