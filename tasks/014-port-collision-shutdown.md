# Bug: Port Collision Does Not Trigger Process Shutdown

**Type:** Task (Bug)
**Status:** In Progress
**Created:** 2026-02-12

## Context

When the platform binary starts and an HTTP server fails to bind its port (e.g., another instance is already running), the error is logged but the process does not exit. Background workers (outbox poller, event consumer) continue running indefinitely, and `main()` blocks forever on the signal channel.

This was discovered when 5 stale platform processes accumulated — each had failed to bind ports 8080/8081 but kept running background workers against the shared database and Redpanda.

## Root Cause

`ingestion.Start()` and `query.Start()` launch `http.ListenAndServe` in a goroutine. The bind error happens asynchronously and is only logged — it is never propagated back to `main()`. The `select` in `main()` only listens for OS signals and context cancellation, not service errors.

## Expected Behavior

If any service's HTTP server fails to start (port conflict, permission denied, etc.), the entire process should initiate graceful shutdown and exit with a non-zero code.

## Proposed Fix

Each `Start()` should return an error channel (or the service should cancel the parent context on fatal errors). `main()` should select on this channel alongside the signal channel:

```go
select {
case sig := <-sigCh:
    slog.Info("received shutdown signal", "signal", sig)
case err := <-errCh:
    slog.Error("service fatal error, shutting down", "error", err)
case <-ctx.Done():
    slog.Info("context cancelled")
}
```

## Scope

- Ingestion Service (HTTP :8080)
- Query Service (HTTP :8081)
- Action Orchestrator (HTTP :8083, future)

## Related

- [Insight: Propagate Async Server Errors to Main](../../platform-docs/insights/development/008-propagate-async-server-errors.md)
