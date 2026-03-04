# Verify Environment Script

**Type:** Task
**Status:** Backlog
**Created:** 2026-02-13

## Context

Setting up the local testing environment requires multiple services to be running and correctly configured (Postgres, Redpanda, correct ports, migrations applied, etc.). When tests fail, it's not always obvious whether the failure is a code bug or an environment misconfiguration. A verification script would give developers a quick way to confirm their local environment is correctly set up before running tests.

## Requirements

Create a `verify-environment.sh` (or `.py`) script that checks all prerequisites for local development and testing. The script should:

### Checks

1. **Docker running** — verify Docker daemon is accessible
2. **Required containers running** — check Postgres and Redpanda containers are up and healthy
3. **Postgres connectivity** — connect to `localhost:5432` with expected credentials, verify the database exists
4. **Redpanda connectivity** — connect to `localhost:9092`, verify broker is reachable
5. **Migrations applied** — check that expected tables exist (outbox, event_store, projections, dlq)
6. **Go toolchain** — verify `go` is installed and the correct version
7. **Port availability** — check that ports 8080/8081 are free (not already bound by another process) when running in skeleton mode

### Output

- Print a checklist with pass/fail for each check, using emojis to indicate status (e.g., ✅ pass, ❌ fail, ⚠️ warning)
- Exit 0 if all checks pass, non-zero otherwise
- Include actionable remediation hints for failures (e.g., "Run `make skeleton-up` to start infrastructure")

### Integration

- Add a `make verify` Makefile target
- Reference from DEVELOPMENT.md as a first step for new developers

## Notes

- Keep it simple — a shell script with `psql`, `curl`, and basic port checks is sufficient. Python only if the logic gets complex enough to warrant it.
- This is a developer productivity tool, not a CI gate.
