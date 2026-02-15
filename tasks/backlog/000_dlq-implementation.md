# DLQ Implementation Status (Event Handler Service)

## Context

The Dead Letter Queue (DLQ) is defined in `design-spec/06-database-schemas.md` (Section 6.3) as a PostgreSQL table intended to store events that fail processing in various services, including the `EventHandler`. This document outlines the current state of its implementation for the `EventHandler` service.

## Current Status

### Database Schema

The DLQ database table for the `EventHandler` service has been successfully defined and created. This is confirmed by the migration file:
`platform-services/internal/services/eventhandler/migrations/002_create_dlq.sql`

The schema of the created table aligns precisely with the definition in `platform-docs/design-spec/06-database-schemas.md`, including columns such as `dlq_id`, `consumer`, `event_id`, `event_payload`, `error_message`, `failed_at`, `retry_count`, and `status`, along with the specified indexes.

### Application Logic

An inspection of the `EventHandler` service's Go code (`platform-services/internal/services/eventhandler/consumer.go`, `platform-services/internal/services/eventhandler/handlers.go`) indicates that while error handling is present and various processing failures (e.g., event deserialization, projection writing) are logged, **there is currently no explicit application logic implemented to write these failed events into the `dlq` table.**

Failures are currently handled by logging the error, but the event is not persisted to the DLQ for later inspection or replay.

## Next Steps / Future Work

-   Implement the logic within the `EventHandler` service to persist failed events to the `dlq` table. This involves:
    *   Identifying points of failure in event consumption and handling.
    *   Creating a DLQ repository interface and implementation.
    *   Integrating the DLQ write operation into the error paths of the `consumer.go` and `handlers.go`.
-   Consider adding similar DLQ implementation for other services that are intended to use a DLQ (e.g., `TSDB Writer`, `Action Orchestrator`).
-   Define a strategy for monitoring and replaying events from the DLQ.
