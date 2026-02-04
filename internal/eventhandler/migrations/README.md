# Event Handler Migrations

Migrations for the Event Handler database (see ADR-0010).

## Tables Owned

| Table | Purpose |
|-------|---------|
| `projections` | Materialized views for queries (CQRS read side) |
| `dlq` | Dead letter queue for failed event processing |

## Migration Files

| File | Description |
|------|-------------|
| `001_create_projections.sql` | Creates projections table |
| `002_create_dlq.sql` | Creates dead letter queue table |

## Running Migrations

```bash
# Via Makefile
make migrate-eventhandler

# Or directly
docker compose exec postgres psql -U cornjacket -d cornjacket -f internal/eventhandler/migrations/001_create_projections.sql
```

## Configuration

Set `EVENTHANDLER_DATABASE_URL` to point to the database for this service.

Default (dev): `postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable`
