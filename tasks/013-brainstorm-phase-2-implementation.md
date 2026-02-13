# Task 013: Brainstorm Phase 2 Implementation

**Type:** Task
**Status:** Complete
**Created:** 2026-02-11

## Context

Phase 1: Local Skeleton is complete. This task captures the brainstorming decisions needed before implementing Phase 2: Local Full Stack. Three key questions were identified and resolved.

## Decisions

### Q1: Where should Docker Compose files live?

**Decision:** Dedicated subdirectory — `platform-services/docker-compose/`.

Reduces root clutter as the number of compose files grows. The slight verbosity in `-f` paths is abstracted by Makefile targets.

### Q2: Should the platform binary be containerized?

**Decision:** Layered Docker Compose overrides — two modes.

- **Skeleton mode** (base file only): Infrastructure in containers, platform binary on host. Fastest iteration for Go development. Matches the current Phase 1 setup.
- **Fullstack mode** (base + overlay): Everything containerized including the platform. Traefik routes HTTP, EMQX handles MQTT. Mirrors the production ECS sidecar deployment.

Two compose files, not four. The brainstorming doc originally proposed separate `docker-compose.monolith.yaml` and `docker-compose.e2e.yaml` files — these were rejected as unnecessary complexity. The skeleton mode doesn't need a compose file for the host binary (it's just `go run`), and e2e test overrides can be added later only if actually needed.

### Q3: How should multiple Docker Compose configurations be managed?

**Decision:** Two compose files combined via `-f` flags, wrapped in Makefile targets. No Docker Compose profiles.

```
docker-compose/
├── docker-compose.yaml            ← Base: Postgres, Redpanda, Console
└── docker-compose.fullstack.yaml  ← Overlay: Traefik, EMQX, containerized platform
```

```makefile
COMPOSE_BASE = -f docker-compose/docker-compose.yaml
COMPOSE_FULL = $(COMPOSE_BASE) -f docker-compose/docker-compose.fullstack.yaml

skeleton-up:    docker compose $(COMPOSE_BASE) up -d
fullstack-up:   docker compose $(COMPOSE_FULL) up -d --build
```

Profiles were rejected — they solve the problem of multiple modes in a single file, but with two separate files and `-f`, you get the same result more simply.

## Follow-Up

Implementation is tracked in [Spec 014: Docker Compose Restructure & Platform Containerization](014-docker-compose-restructure.md).

## Verification

- [x] All three brainstorming questions resolved with clear decisions
- [x] Spec 014 created to track implementation
- [x] Update PROJECT.md to reflect task completion
