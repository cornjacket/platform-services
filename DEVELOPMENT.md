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
│   ├── client/                      # Service client libraries
│   │   └── eventhandler/            # Client for submitting events to EventHandler
│   │       └── client.go            # SubmitEvent() - wraps Redpanda publish
│   │
│   ├── shared/                      # Shared code (config, domain, infrastructure)
│   │   ├── config/
│   │   │   └── config.go            # Env vars, feature flags
│   │   ├── domain/
│   │   │   ├── clock/               # Time abstraction for testability
│   │   │   │   └── clock.go         # RealClock, FixedClock, ReplayClock
│   │   │   ├── events/              # Event types and envelope
│   │   │   │   └── envelope.go
│   │   │   └── models/              # Domain models
│   │   ├── projections/             # Shared projection store
│   │   │   ├── store.go             # Store interface and Projection type
│   │   │   └── postgres.go          # PostgreSQL implementation
│   │   └── infra/                   # Infrastructure adapters
│   │       ├── postgres/
│   │       │   ├── client.go        # Connection pool, health check
│   │       │   ├── outbox.go        # OutboxRepository implementation
│   │       │   └── eventstore.go    # EventStoreWriter implementation
│   │       └── redpanda/
│   │           └── producer.go      # Kafka producer wrapper
│   │
│   └── services/                    # Individual services
│       ├── ingestion/               # Ingestion Service (:8080)
│       │   ├── migrations/          # Service-owned migrations (ADR-0010)
│       │   │   ├── 001_create_outbox.sql
│       │   │   └── 002_create_event_store.sql
│       │   ├── handler.go           # HTTP handlers
│       │   ├── service.go           # Business logic
│       │   ├── repository.go        # Interface definitions
│       │   ├── routes.go
│       │   └── worker/              # Background worker (outbox processor)
│       │       ├── processor.go     # Reads outbox, writes event store, submits to EventHandler
│       │       └── repository.go    # Worker interfaces
│       │
│       ├── query/                   # Query Service (:8081)
│       │   ├── handler.go           # HTTP handlers
│       │   ├── service.go           # Business logic
│       │   ├── repository.go        # Interface definitions
│       │   └── routes.go
│       │
│       ├── eventhandler/            # Event Handler (background worker)
│       │   ├── migrations/
│       │   │   ├── 001_create_projections.sql
│       │   │   └── 002_create_dlq.sql
│       │   ├── consumer.go          # Kafka consumer
│       │   ├── handlers.go          # Event dispatch and handlers
│       │   └── repository.go        # Interface definitions
│       │
│       ├── actions/                 # Action Orchestrator (:8083) - future
│       │
│       └── tsdb/                    # TSDB Writer (optional) - future
│
├── pkg/                             # Public libraries (importable by other repos)
│   └── client/                      # Go client SDK for the platform - future
│
├── api/                             # API definitions
│   └── openapi/
│
├── e2e/                             # End-to-end tests
│   ├── runner/                      # Test framework
│   ├── client/                      # HTTP client helpers
│   └── tests/                       # Test implementations
│
├── deploy/                          # Deployment code
│   └── terraform/
│
├── docker-compose/
│   ├── docker-compose.yaml          # Base: infrastructure (Postgres, Redpanda)
│   └── docker-compose.fullstack.yaml # Overlay: adds Traefik, EMQX, containerized platform
├── Dockerfile                       # Multi-stage Go build for platform container
├── .dockerignore                    # Excludes test files, docs from build context
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

- Go 1.25+
- Docker with Compose plugin (`docker compose`)
- Make (optional, for convenience commands)

### Two Modes

The project supports two Docker Compose modes:

| Mode | Command | Platform runs as | Use case |
|------|---------|-----------------|----------|
| **Skeleton** | `make skeleton-up` | Binary on host | Fast Go iteration, debugging |
| **Fullstack** | `make fullstack-up` | Container (behind Traefik) | Production fidelity, routing tests |

Skeleton mode is the default for day-to-day development.

### Running Locally (Skeleton Mode)

1. Start infrastructure:
   ```bash
   make skeleton-up
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

### Running Locally (Fullstack Mode)

1. Start everything:
   ```bash
   make fullstack-up
   ```

2. Access points:
   - Platform (via Traefik): http://localhost
   - Traefik dashboard: http://localhost:8090
   - EMQX dashboard: http://localhost:18083
   - MQTT broker: localhost:1883

### Starting Fresh

To reset the database and start with a clean slate:

```bash
# Stop containers and remove volumes (destroys all data)
make skeleton-down
docker compose -f docker-compose/docker-compose.yaml down -v

# Start fresh
make skeleton-up
make migrate-all
```

### Testing the Ingestion Endpoint

```bash
# Ingest an event
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"event_type":"sensor.reading","aggregate_id":"device-001","payload":{"value":72.5,"unit":"fahrenheit"}}'

# Expected response:
# {"event_id":"<uuid>","status":"accepted"}

# Verify the event is in the outbox (should be empty after processing)
docker compose -f docker-compose/docker-compose.yaml exec postgres psql -U cornjacket -d cornjacket -c "SELECT * FROM outbox;"

# Check health endpoint
curl http://localhost:8080/health
```

**Note:** When using backslash line continuation in zsh/bash, ensure there are no trailing spaces after `\`.

### Testing End-to-End Event Flow

The full event flow: HTTP → Ingestion → Outbox → Event Store + Redpanda → Event Handler → Projections

```bash
# 1. Ingest a sensor.reading event
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"event_type":"sensor.reading","aggregate_id":"device-001","payload":{"value":72.5,"unit":"fahrenheit"}}'

# 2. Ingest a user.login event
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"event_type":"user.login","aggregate_id":"user-123","payload":{"user_id":"user-123","ip":"192.168.1.1"}}'

# 3. Wait for processing (typically < 1 second)
sleep 2

# 4. Verify projections were created
docker compose -f docker-compose/docker-compose.yaml exec postgres psql -U cornjacket -d cornjacket -c \
  "SELECT projection_type, aggregate_id, state FROM projections ORDER BY projection_type;"

# Expected output:
#  projection_type | aggregate_id |                    state
# -----------------+--------------+----------------------------------------------
#  sensor_state    | device-001   | {"unit": "fahrenheit", "value": 72.5}
#  user_session    | user-123     | {"ip": "192.168.1.1", "user_id": "user-123"}

# 5. Verify event is in event_store
docker compose -f docker-compose/docker-compose.yaml exec postgres psql -U cornjacket -d cornjacket -c \
  "SELECT event_id, event_type, aggregate_id FROM event_store ORDER BY timestamp DESC LIMIT 5;"

# 6. Verify outbox is empty (all events processed)
docker compose -f docker-compose/docker-compose.yaml exec postgres psql -U cornjacket -d cornjacket -c \
  "SELECT COUNT(*) FROM outbox;"
# Expected: 0

# 7. Test projection update (send newer event for same aggregate)
curl -X POST http://localhost:8080/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"event_type":"sensor.reading","aggregate_id":"device-001","payload":{"value":75.0,"unit":"fahrenheit"}}'

sleep 2

# 8. Verify projection was updated
docker compose -f docker-compose/docker-compose.yaml exec postgres psql -U cornjacket -d cornjacket -c \
  "SELECT state FROM projections WHERE projection_type = 'sensor_state' AND aggregate_id = 'device-001';"
# Expected: {"unit": "fahrenheit", "value": 75.0}
```

### Checking Redpanda Messages

```bash
# List topics
docker compose -f docker-compose/docker-compose.yaml exec redpanda rpk topic list

# Consume messages from sensor-events topic
docker compose -f docker-compose/docker-compose.yaml exec redpanda rpk topic consume sensor-events --num 5

# Consume messages from user-actions topic
docker compose -f docker-compose/docker-compose.yaml exec redpanda rpk topic consume user-actions --num 5
```

### Running Tests

```bash
# Unit tests
make test

# With coverage
make test-coverage

# Integration tests (requires make skeleton-up)
make test-integration

# Component tests (requires make skeleton-up)
make test-component

# All tests
make test-all

# E2E tests against skeleton (requires make dev)
make e2e-skeleton

# E2E tests against fullstack (requires make fullstack-up)
make e2e-fullstack
```

## Configuration

Configuration is loaded from environment variables with the naming convention `CJ_[SERVICE]_[VARIABLE_NAME]`.

**Complete reference:** See [design-spec.md section 12](../platform-docs/design-spec.md#12-environment-variables) for all variables, defaults, and per-environment values.

### Local Development Defaults

All defaults are configured for local development — no environment variables needed to run locally.

| Variable | Default | Description |
|----------|---------|-------------|
| `CJ_INGESTION_PORT` | 8080 | Ingestion service port |
| `CJ_QUERY_PORT` | 8081 | Query service port |
| `CJ_ACTIONS_PORT` | 8083 | Actions service port |
| `CJ_INGESTION_DATABASE_URL` | localhost:5432/cornjacket | PostgreSQL connection |
| `CJ_REDPANDA_BROKERS` | localhost:9092 | Kafka broker addresses |

### Overriding Configuration

To override any setting, export the environment variable before running:

```bash
# Example: change ingestion port
export CJ_INGESTION_PORT=9080
go run ./cmd/platform

# Example: use different database
export CJ_INGESTION_DATABASE_URL="postgres://user:pass@host:5432/db?sslmode=require"
go run ./cmd/platform
```

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

## Task Management Process

The development workflow for this project is driven by explicit task documents. These documents are categorized as "Specs" (for new features or significant changes) or "Tasks" (for bug fixes or minor improvements).

All task documents are managed within the `tasks/` directory, with a dedicated `tasks/README.md` serving as the **single source of truth for task numbering and indexing**.

### Key Principles:

1.  **Task Document First**: Every change to the codebase *must* have an accompanying task document (Spec or Task).
2.  **Sequential Numbering**: Task documents are numbered sequentially (e.g., `001-description.md`, `002-description.md`).
3.  **`tasks/README.md` Index**: This `README.md` contains the authoritative list of all active and completed tasks. When creating a new task:
    *   **Always consult the `## Index` section** in `tasks/README.md` to find the highest sequential number.
    *   **Assign the next available number** (`Highest + 1`) to your new task.
    *   **Immediately add the new task to the `## Index` section** of `tasks/README.md` upon creation, before writing the actual task file content. This reserves the number and keeps the index up-to-date.
    *   Task file names (`NNN-description.md`) *must* match the number and description in this index to maintain consistency.
4.  **Backlog Management**: For potential future tasks not yet committed for immediate implementation, use the `tasks/backlog/` directory. These are indexed in `tasks/BACKLOG.md`. When a backlog item is promoted to an active task, it follows the numbering process described above.
5.  **`PROJECT.md` Updates**: For tasks that impact the overall project status, remember to update `platform-docs/PROJECT.md` to reflect task completion. This is typically done for milestones or significant deliverables.

This structured approach ensures clear communication of project progress and maintains a consistent, traceable development history.

## Definition of Done for a Development Task

A development task (whether a Spec or a Task) is considered "Done" when **all** of the following criteria are met:

1.  **Code Complete:** All functional requirements of the task are implemented.
2.  **Tests Pass:**
    *   **Unit Tests:** All `make test` pass.
    *   **Integration Tests:** All `make test-integration` pass (if applicable to the changes).
    *   **Component Tests:** All `make test-component` pass (if applicable to the changes).
    *   **E2E Tests:** All `make e2e-skeleton` and/or `make e2e-fullstack` pass for relevant scenarios (if applicable to the changes).
    *   *(Note: The `make test-all` target can be used to run all tiers of tests conveniently.)*
3.  **Code Reviewed:** (Future) Code has been formally reviewed and approved (implies a Pull Request process, which will be defined in a future phase).
4.  **Documentation Updated:**
    *   The task document (`NNN-description.md`) status is set to "Complete."
    *   The `platform-services/tasks/README.md` index is updated (if a new task or status change occurred).
    *   `platform-docs/PROJECT.md` is updated to reflect task completion (if the task is a milestone or listed in "Current Focus").
    *   Relevant inline code comments, API docs, design documents (`design-spec.md`), and architectural records (`ARCHITECTURE.md`, ADRs) are up-to-date and reflect the changes.
5.  **Insights Captured:** Any new insights, patterns, or lessons discovered during the task's implementation are recorded in `platform-docs/insights/` or `ai-builder-lessons/`.

This comprehensive definition ensures that tasks are not just "coded," but truly completed to a high standard, contributing to the overall quality and maintainability of the project.

## Related Documentation

- [Project Plan](../platform-docs/PROJECT.md) — Current phase and milestones
- [ADRs](../platform-docs/decisions/) — Architectural decisions
- [Design Spec](../platform-docs/design-spec.md) — Operational parameters
- [Architecture](ARCHITECTURE.md) — Clean Architecture alignment
- [Task Documents](tasks/) — Feature implementation specs
