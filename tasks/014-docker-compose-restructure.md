# Spec 014: Docker Compose Restructure & Platform Containerization

**Type:** Spec
**Status:** Draft
**Created:** 2026-02-11

## Context

Phase 2: Local Full Stack requires running the platform as a container alongside Traefik and EMQX. The current setup has a single `docker-compose.yml` in the project root that provides infrastructure only (Postgres, Redpanda). The platform binary runs on the host.

This spec restructures Docker Compose into a layered configuration and containerizes the platform binary, enabling both fast local development (skeleton mode) and production-fidelity testing (fullstack mode).

Decisions were made in [Task 013](013-brainstorm-phase-2-implementation.md).

## Current Docker Compose Layering

Today, a single `docker-compose.yml` serves all test tiers:

```
┌─────────────────────────────────────────────────┐
│ docker-compose.yml (Infrastructure)             │
│  - postgres (TimescaleDB, port 5432)            │
│  - redpanda (Kafka-compatible, port 9092)       │
│  - redpanda-console (UI, port 8084)             │
└─────────────────────────────────────────────────┘
         │
         ├── Integration tests:  go test -tags=integration
         │   (test code connects directly to postgres/redpanda)
         │
         ├── Component tests:    go test -tags=component
         │   (Start() spins up service in-process, connects to postgres/redpanda)
         │
         └── E2E tests:          go run e2e/main.go
             (platform binary runs on host, tests hit HTTP endpoints)
```

The platform binary always runs **on the host**. Docker only provides infrastructure.

## Phase 2 Layered Configuration

Two compose files, combined with `-f` flags:

```
docker-compose/
├── docker-compose.yaml            ← Base: infrastructure only (what we have today)
└── docker-compose.fullstack.yaml  ← Overlay: adds Traefik, EMQX, containerized platform
```

### Skeleton Mode (developer iteration)

```
make skeleton-up    →  docker compose -f docker-compose/docker-compose.yaml up -d
```

Same as today: infrastructure in containers, platform binary on host. Fastest feedback loop for Go development.

Used by: integration tests, component tests, e2e tests (binary mode).

### Fullstack Mode (production fidelity)

```
make fullstack-up   →  docker compose -f docker-compose/docker-compose.yaml \
                                       -f docker-compose/docker-compose.fullstack.yaml up -d --build
```

Everything in containers. Traefik routes HTTP, EMQX handles MQTT, platform runs as a container on the Docker network. Mirrors the ECS sidecar deployment.

Used by: e2e tests (fullstack mode), manual integration testing.

### Networking

- **Skeleton mode:** Tests and the host binary connect to `localhost:{port}` (same as today).
- **Fullstack mode:** All services communicate on the Docker bridge network. Traefik is the external entry point. E2E tests hit Traefik's exposed port instead of the platform directly.

## Functionality

### 1. Subdirectory: `docker-compose/`

Move `docker-compose.yml` from project root to `docker-compose/docker-compose.yaml`:

- Rename `.yml` → `.yaml` (canonical extension)
- No content changes to the base file (Postgres, Redpanda, Redpanda Console stay identical)

### 2. Dockerfile

Create `Dockerfile` in project root (standard multi-stage Go build):

- **Builder stage:** `golang:1.23` (match go.mod version), compile static binary
- **Runtime stage:** `gcr.io/distroless/static-debian12` (minimal, no shell)
- Binary entrypoint: `/platform`
- Expose ports 8080 (ingestion), 8081 (query), 8082 (action orchestrator)

### 3. Fullstack Compose Overlay: `docker-compose/docker-compose.fullstack.yaml`

Adds to the base:

| Service | Image | Purpose | Exposed Port |
|---------|-------|---------|-------------|
| `platform` | Built from `Dockerfile` | Containerized monolith | 8080, 8081 (via Traefik) |
| `traefik` | `traefik:v3.x` | HTTP reverse proxy / gateway | 80 |
| `emqx` | `emqx/emqx:5.x` | MQTT broker | 1883, 18083 (dashboard) |

The `platform` service:
- Builds from `../Dockerfile` (context is project root)
- Connects to `postgres` and `redpanda` via Docker network hostnames
- Environment variables override connection strings (e.g., `DB_HOST=postgres`, `REDPANDA_BROKERS=redpanda:9092`)
- Depends on `postgres`, `redpanda`

### 4. Makefile Updates

Replace existing docker targets and add new ones:

```makefile
COMPOSE_DIR  = docker-compose
COMPOSE_BASE = -f $(COMPOSE_DIR)/docker-compose.yaml
COMPOSE_FULL = $(COMPOSE_BASE) -f $(COMPOSE_DIR)/docker-compose.fullstack.yaml

# Skeleton (infrastructure only — platform runs on host)
skeleton-up:        docker compose $(COMPOSE_BASE) up -d
skeleton-down:      docker compose $(COMPOSE_BASE) down
skeleton-logs:      docker compose $(COMPOSE_BASE) logs -f

# Fullstack (everything containerized)
fullstack-up:       docker compose $(COMPOSE_FULL) up -d --build
fullstack-down:     docker compose $(COMPOSE_FULL) down
fullstack-logs:     docker compose $(COMPOSE_FULL) logs -f

# Container build (platform image only, no compose)
docker-build:       docker build -t cornjacket-platform .

# Migration targets updated to use COMPOSE_BASE paths
# E2E targets for both modes (see section 5)
```

Deprecate `docker-up` / `docker-down` in favor of explicit `skeleton-up` / `fullstack-up`. Remove `docker-up` / `docker-down` / `docker-logs` after migration.

The `dev` target becomes: `skeleton-up migrate-all run`.

### 5. E2E Tests on Both Modes

The e2e runner already supports URL configuration via `E2E_INGESTION_URL` and `E2E_QUERY_URL` environment variables (see `e2e/runner/runner.go:LoadConfig`). No changes to the e2e runner code are needed.

**Skeleton e2e** (binary on host — same as today):

```makefile
e2e-skeleton:
	@echo "Running e2e tests against skeleton (binary on host)..."
	E2E_INGESTION_URL=http://localhost:8080 \
	E2E_QUERY_URL=http://localhost:8081 \
	go run ./e2e -env local
```

Requires: `make skeleton-up`, `make migrate-all`, `make run` (or `make dev`).

**Fullstack e2e** (containerized platform behind Traefik):

```makefile
e2e-fullstack:
	@echo "Running e2e tests against fullstack (containerized)..."
	E2E_INGESTION_URL=http://localhost/ingest \
	E2E_QUERY_URL=http://localhost/query \
	go run ./e2e -env local
```

Requires: `make fullstack-up` (migrations run inside the container or via init container).

> **Note:** The exact Traefik URL paths (`/ingest`, `/query`) depend on Traefik routing rules defined in the fullstack overlay. These are illustrative — final paths will be set when Traefik is configured.

**Requirement:** Both `make e2e-skeleton` and `make e2e-fullstack` must pass. The skeleton target validates the Go binary; the fullstack target validates the containerized deployment. If either fails, the change is not complete.

## Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `docker-compose/docker-compose.yaml` | Create (move from root) | Base infrastructure |
| `docker-compose/docker-compose.fullstack.yaml` | Create | Fullstack overlay (Traefik, EMQX, platform container) |
| `Dockerfile` | Create | Multi-stage Go build for platform binary |
| `.dockerignore` | Create | Exclude test files, docs, bin/ from build context |
| `Makefile` | Modify | New targets, updated paths |
| `docker-compose.yml` | Delete | Replaced by `docker-compose/docker-compose.yaml` |
| `platform-docs/PROJECT.md` | Modify | Mark task complete in Phase 2 checklist, update Current Focus |
| `platform-docs/design-spec.md` | Modify | Document skeleton/fullstack modes and compose layering |
| `platform-services/DEVELOPMENT.md` | Modify | Update docker commands to reflect new targets (`skeleton-up`, `fullstack-up`) |

## Acceptance Criteria

- [ ] `docker-compose.yml` removed from project root
- [ ] `docker-compose/docker-compose.yaml` contains base infrastructure (Postgres, Redpanda, Console)
- [ ] `docker-compose/docker-compose.fullstack.yaml` adds Traefik, EMQX, containerized platform
- [ ] `Dockerfile` builds a working platform binary image
- [ ] `.dockerignore` excludes non-essential files from build context
- [ ] `make skeleton-up` starts infrastructure only (matches current `make docker-up` behavior)
- [ ] `make fullstack-up` starts all services including containerized platform
- [ ] `make docker-build` builds the platform container image
- [ ] `make e2e-skeleton` runs e2e tests against host binary (existing behavior preserved)
- [ ] `make e2e-fullstack` runs e2e tests against containerized platform behind Traefik
- [ ] Integration tests still work: `make skeleton-up && make test-integration`
- [ ] Component tests still work: `make skeleton-up && make test-component`
- [ ] Migration targets work with new compose paths
- [ ] `docker-up` / `docker-down` removed (no legacy aliases)
- [ ] `platform-docs/design-spec.md` documents skeleton/fullstack modes and compose layering
- [ ] `DEVELOPMENT.md` updated with new `make` targets and workflow
- [ ] Update `platform-docs/PROJECT.md` to reflect task completion

## Notes

- Traefik and EMQX configuration details (routing rules, MQTT topics) will be addressed in their own specs. This spec only adds them as services in the fullstack overlay with minimal configuration.
- The fullstack overlay's `platform` service environment variables must match what `cmd/platform/main.go` expects via `config.Load()`. Verify the config package supports the required env vars.
- Container migrations: In fullstack mode, migrations need to run inside the Docker network. Options include an init container, a `migrate` service in the compose file, or running migrations from the host against the exposed Postgres port. Decide during implementation.
