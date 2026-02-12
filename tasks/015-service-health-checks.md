# Service Health Check Endpoints

**Type:** Task
**Status:** In Progress
**Created:** 2026-02-12

## Context

Currently only the Ingestion Service exposes a `/health` endpoint. All services in the platform should expose HTTP health check endpoints for:

- Docker Compose `healthcheck` directives (container orchestration)
- AWS ECS task health checks
- Load balancer target group health checks (Traefik, ALB)
- Operational monitoring and alerting

## Requirements

Every service must expose a `GET /health` HTTP endpoint, including background workers. Process liveness alone is insufficient — ECS and Kubernetes health checks are HTTP-based, and a live process can still be deadlocked, disconnected from Kafka, or holding a hung database connection. Each endpoint must return:

- **200 OK** when the service is ready to accept traffic
- **503 Service Unavailable** when the service is degraded or not ready

### Services Requiring Health Checks

| Service | Port | Current Status |
|---------|------|----------------|
| Ingestion | 8080 | Has `/health` (basic, always returns 200) |
| Query | 8081 | No health endpoint |
| Action Orchestrator | 8083 | Not yet implemented |
| Event Handler | 8084 | No health endpoint (needs minimal HTTP server) |

### Health Check Depth

**Shallow (Phase 1):** Return 200 if the HTTP server is listening. This is what Ingestion currently does.

**Deep (Phase 2, optional):** Verify downstream dependencies (database connectivity, Redpanda reachability). Return 503 with a JSON body describing which checks failed. This is more useful for debugging but adds latency to health checks.

### Event Handler

The Event Handler is a background consumer with no current HTTP server. It must add a minimal HTTP server solely for the health check endpoint. The health response should reflect actual consumer readiness — not just "process is alive" but "I am connected to Kafka and processing."

### Response Format

```json
{
  "status": "ok",
  "service": "ingestion",
  "uptime_seconds": 3600
}
```

## Related

- [Spec 014: Docker Compose Restructure](../014-docker-compose-restructure.md) — Traefik routes based on service health
- [Backlog 002: Port Collision Shutdown](002_port-collision-shutdown.md) — Health checks are meaningless if bind failures don't trigger shutdown
