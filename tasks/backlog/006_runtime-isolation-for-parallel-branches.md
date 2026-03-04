# Runtime Isolation for Parallel Feature Branches

**Type:** Task
**Status:** Backlog
**Created:** 2026-02-13

## Context

Git worktrees (Task 001 in platform-docs) provide code-level isolation for parallel feature work. However, runtime resources are shared and will conflict if two branches attempt to run locally simultaneously.

## Problem

Shared resources when running `docker-compose up` from multiple worktrees:

| Resource | Conflict |
|----------|----------|
| Host ports (8080, 8081, 8082, 5432, 9092, ...) | `bind: address already in use` |
| Docker container names | Name collisions |
| Docker volumes (PostgreSQL data, Redpanda data) | Data corruption / cross-contamination |
| Docker network names | Name collisions |

## Possible Approaches

- **Per-branch port offsets** — env var (e.g., `PORT_OFFSET=100`) shifts all ports
- **Dynamic compose project names** — `COMPOSE_PROJECT_NAME=$BRANCH` namespaces containers, volumes, networks
- **Only one branch runs at a time** — simplest; enforce via a lock file or convention

## Requirements

- Two feature branches must be able to run their full local stack without conflicts
- Or: explicitly document and enforce a single-branch-running convention

## Notes

- `COMPOSE_PROJECT_NAME` solves container/volume/network naming but not host port conflicts
- Port offsets require all service configs to be parameterized
- See ai-builder-lessons/017 for broader context
