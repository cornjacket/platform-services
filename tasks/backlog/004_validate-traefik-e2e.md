# Validate Traefik Routing via E2E Tests

**Type:** Task
**Status:** Backlog
**Created:** 2026-02-13

## Context

Traefik is already configured in `docker-compose/docker-compose.fullstack.yaml` with path-based routing:
- `/api/v1/events` and `/health` → ingestion:8080
- `/api/v1/projections` → query:8081

The `e2e-fullstack` Makefile target points both URLs through `http://localhost` (port 80, Traefik). However, this flow has never been validated — nobody has confirmed `make fullstack-up` + `make e2e-fullstack` actually passes.

Additionally, `e2e/run.sh` only supports `local` (skeleton) mode with hardcoded direct-port URLs. It has no native awareness of fullstack/Traefik mode.

## Requirements

### 1. Add `local-fullstack` environment to `run.sh`

Add a new environment option so `./run.sh -env=local-fullstack` sets both URLs to `http://localhost` (port 80, through Traefik), matching what the Makefile's `e2e-fullstack` target already does.

### 2. Validate existing e2e tests pass through Traefik

Run `make fullstack-up` + `make e2e-fullstack` and fix whatever breaks. The three existing tests (ingest-event, query-projection, full-flow) should all pass when routed through Traefik.

### 3. (Optional) Add Traefik-specific e2e test

Consider a test that validates Traefik-specific behavior:
- `/health` routes correctly through Traefik
- Unknown paths return an appropriate error (Traefik 404 vs service 404)

## Dependency

**Blocked by:** [Backlog 003: Service Health Check Endpoints](003_service-health-checks.md)

After backlog 003 is complete (health endpoints on all services), the fullstack docker-compose will need updates:
- Add `healthcheck` directives on the `platform` container using the new health endpoints
- Update Traefik `depends_on` to wait for platform health
- Potentially add health check routing for Query and Event Handler services

These docker-compose changes should be done as part of this task or coordinated with it.

## Affected Files

- `e2e/run.sh` — add `local-fullstack` environment
- `docker-compose/docker-compose.fullstack.yaml` — potential fixes if e2e tests reveal issues; health check directives after backlog 003
- `e2e/tests/` — optional new Traefik-specific test

## Notes

- This is primarily a validation task, not a feature build. Traefik config already exists.
- Keep the existing `e2e-skeleton` flow working — both modes must coexist.
