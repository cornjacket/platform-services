# Ingestion Service Migrations

Migrations for the Ingestion Service database (see ADR-0010).

## Tables Owned

| Table | Purpose |
|-------|---------|
| `outbox` | Temporary holding area for outbox-first write pattern |
| `event_store` | Append-only log of all events (CQRS write side) |

## Migration Files

| File | Description |
|------|-------------|
| `001_create_outbox.sql` | Creates outbox table with NOTIFY trigger |
| `002_create_event_store.sql` | Creates event_store table |

## Running Migrations

```bash
# Via Makefile
make migrate-ingestion

# Or directly
docker compose exec postgres psql -U cornjacket -d cornjacket -f internal/ingestion/migrations/001_create_outbox.sql
```

## Configuration

Set `INGESTION_DATABASE_URL` to point to the database for this service.

Default (dev): `postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable`
