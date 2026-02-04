# Platform Services - Development Guide

This document describes the project structure, build patterns, and conventions for the platform-services codebase.

## Project Structure

```
platform-services/
├── cmd/
│   └── platform/                    # Single binary entry point
│       └── main.go                  # Wires everything, starts servers + workers
│
├── internal/
│   ├── shared/                      # Shared code (config, domain, infrastructure)
│   │   ├── config/
│   │   │   └── config.go            # Env vars, feature flags
│   │   ├── domain/
│   │   │   ├── events/              # Event types and envelope
│   │   │   │   └── envelope.go
│   │   │   └── models/              # Domain models
│   │   └── infra/                   # Infrastructure adapters
│   │       ├── postgres/
│   │       │   ├── client.go        # Connection pool, health check
│   │       │   └── outbox.go        # OutboxRepository implementation
│   │       ├── redpanda/
│   │       │   ├── producer.go      # Kafka producer wrapper
│   │       │   └── consumer.go      # Kafka consumer wrapper
│   │       └── http/
│   │           └── webhook.go       # HTTP client for webhook delivery
│   │
│   └── services/                    # Individual services
│       ├── ingestion/               # Ingestion Service (:8080)
│       │   ├── migrations/          # Service-owned migrations (ADR-0010)
│       │   │   ├── 001_create_outbox.sql
│       │   │   └── 002_create_event_store.sql
│       │   ├── handler.go           # HTTP handlers
│       │   ├── service.go           # Business logic
│       │   ├── repository.go        # Interface definitions
│       │   └── routes.go
│       │
│       ├── query/                   # Query Service (:8081)
│       │
│       ├── actions/                 # Action Orchestrator (:8083)
│       │
│       ├── eventhandler/            # Event Handler (background worker)
│       │   ├── migrations/
│       │   │   ├── 001_create_projections.sql
│       │   │   └── 002_create_dlq.sql
│       │   └── ...
│       │
│       ├── outbox/                  # Outbox Processor (background worker)
│       │
│       └── tsdb/                    # TSDB Writer (optional)
│
├── pkg/                             # Public libraries (importable by other repos)
│   └── client/                      # Go client SDK for the platform
│
├── api/                             # API definitions
│   └── openapi/
│
├── deploy/                          # Deployment code
│   └── terraform/
│
├── docker-compose.yml               # Local dev (infrastructure only)
├── Dockerfile                       # Production image
├── Makefile                         # Common commands
├── go.mod
├── go.sum
└── DEVELOPMENT.md                   # This file
```

## Dependency Rules

### The Repository Pattern

Services define interfaces for what they need. Infrastructure implements those interfaces.

**Principle:** Services own their interfaces. Infrastructure adapts to them.

#### Example

**internal/services/ingestion/repository.go** — Service defines what it needs:
```go
package ingestion

import "context"

// OutboxRepository defines what the ingestion service needs for outbox writes.
// This interface is owned by ingestion, not by infra.
type OutboxRepository interface {
    Insert(ctx context.Context, event Event) error
}
```

**internal/services/ingestion/service.go** — Service uses the interface:
```go
package ingestion

type Service struct {
    outbox OutboxRepository  // interface, not concrete type
}

func NewService(outbox OutboxRepository) *Service {
    return &Service{outbox: outbox}
}

func (s *Service) Ingest(ctx context.Context, event Event) error {
    // validate...
    return s.outbox.Insert(ctx, event)
}
```

**internal/shared/infra/postgres/outbox.go** — Infrastructure implements the interface:
```go
package postgres

import "github.com/cornjacket/platform-services/internal/services/ingestion"

// OutboxRepo implements ingestion.OutboxRepository
type OutboxRepo struct {
    db *pgxpool.Pool
}

func (r *OutboxRepo) Insert(ctx context.Context, event ingestion.Event) error {
    _, err := r.db.Exec(ctx, "INSERT INTO outbox ...")
    return err
}
```

**cmd/platform/main.go** — Wiring at the entry point:
```go
// Create concrete implementation
outboxRepo := postgres.NewOutboxRepo(dbPool)

// Inject into service (interface satisfied)
ingestionService := ingestion.NewService(outboxRepo)
```

### Dependency Direction

```
cmd/platform (wiring)
      │
      ▼
internal/shared/infra/postgres ───implements───► internal/services/ingestion.OutboxRepository
                                                              ▲
                                                              │
                                              internal/services/ingestion.Service uses
```

**Key rules:**
- `internal/shared/domain` has NO external dependencies (pure Go types)
- Services (`internal/services/*`) depend only on `shared/domain` and their own interfaces
- `internal/shared/infra` imports services to implement their interfaces
- `cmd/platform` wires everything together
- No circular dependencies

### Why This Matters

| Benefit | Explanation |
|---------|-------------|
| **Testability** | Inject mock repositories in tests — no database needed |
| **Swappability** | Replace Postgres with another store without touching service code |
| **Clear contracts** | The interface documents exactly what the service needs |
| **Compile-time safety** | Go verifies implementations satisfy interfaces |
| **No circular deps** | `infra` imports services, not the other way around |

## Local Development

### Prerequisites

- Go 1.21+
- Docker with Compose plugin (`docker compose`)
- Make (optional, for convenience commands)

### Running Locally

1. Start infrastructure:
   ```bash
   docker compose up -d
   ```

2. Run migrations:
   ```bash
   make migrate-all
   ```

3. Run the application:
   ```bash
   go run ./cmd/platform
   ```

   Or use the dev shortcut (does all of the above):
   ```bash
   make dev
   ```

4. Access points:
   - Ingestion API: http://localhost:8080
   - Query API: http://localhost:8081
   - Actions API: http://localhost:8083

### Starting Fresh

To reset the database and start with a clean slate:

```bash
# Stop containers and remove volumes (destroys all data)
docker compose down -v

# Start fresh
docker compose up -d
make migrate-all
```

### Testing the Ingestion Endpoint

```bash
# Ingest an event
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"event_type":"sensor.reading","aggregate_id":"device-001","payload":{"temperature":72.5}}'

# Expected response:
# {"event_id":"<uuid>","status":"accepted"}

# Verify the event is in the outbox
docker compose exec postgres psql -U cornjacket -d cornjacket -c "SELECT * FROM outbox;"

# Check health endpoint
curl http://localhost:8080/health
```

**Note:** When using backslash line continuation in zsh/bash, ensure there are no trailing spaces after `\`.

### Running Tests

```bash
# Unit tests
go test ./...

# With coverage
go test -cover ./...

# Component tests (requires docker compose up)
go test -tags=component ./...
```

## Configuration

Configuration is loaded from environment variables. See `internal/shared/config/config.go` for all options.

### Server Ports

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT_INGESTION` | 8080 | Ingestion service port |
| `PORT_QUERY` | 8081 | Query service port |
| `PORT_ACTIONS` | 8083 | Actions service port |

### Per-Service Database URLs (ADR-0010)

Each service receives its own database URL. In dev, all default to the same database.

| Variable | Default | Service |
|----------|---------|---------|
| `INGESTION_DATABASE_URL` | localhost cornjacket | Ingestion + Outbox Processor |
| `EVENTHANDLER_DATABASE_URL` | localhost cornjacket | Event Handler |
| `QUERY_DATABASE_URL` | localhost cornjacket | Query Service |
| `TSDB_DATABASE_URL` | localhost cornjacket | TSDB Writer |
| `ACTIONS_DATABASE_URL` | localhost cornjacket | Action Orchestrator |

Default: `postgres://cornjacket:cornjacket@localhost:5432/cornjacket?sslmode=disable`

### Other Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `REDPANDA_BROKERS` | localhost:9092 | Kafka broker addresses |
| `ENABLE_TSDB` | false | Enable TSDB writer |

## Coding Conventions

### Error Handling

- Wrap errors with context: `fmt.Errorf("failed to insert event: %w", err)`
- Use structured logging for errors (include event ID, service name)
- Don't log and return — do one or the other

### Logging

- Use structured JSON logging (slog or zerolog)
- Include trace IDs when available
- Log at appropriate levels:
  - ERROR: Something failed that shouldn't
  - WARN: Recoverable issues
  - INFO: Key business events
  - DEBUG: Detailed troubleshooting

### Naming

- Interfaces: describe capability (e.g., `OutboxRepository`, `EventPublisher`)
- Implementations: describe technology (e.g., `PostgresOutboxRepo`, `RedpandaProducer`)
- Files: lowercase, match primary type (e.g., `repository.go`, `service.go`)

## Related Documentation

- [Project Plan](../platform-docs/PROJECT.md) — Current phase and milestones
- [ADRs](../platform-docs/decisions/) — Architectural decisions
- [Design Spec](../platform-docs/design-spec.md) — Operational parameters
