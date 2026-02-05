# Task 003: Event Handler

**Status:** Draft
**Created:** 2026-02-05
**Updated:** 2026-02-05

## Context

Phase 1 of the platform requires end-to-end event flow:

```
HTTP Request → Ingestion → Outbox → Event Store + Redpanda → Consumer
                                                              ▲
                                                              └── This task
```

The Outbox Processor publishes events to Redpanda topics. The Event Handler consumes these events and updates projections (CQRS read side).

See:
- [ADR-0003 CQRS with Eventual Consistency](../../platform-docs/decisions/0003-cqrs-with-eventual-consistency.md)
- [ADR-0014 Event Handler Consumer Strategy](../../platform-docs/decisions/0014-event-handler-consumer-strategy.md)
- [Design Spec — Event Types](../../platform-docs/design-spec.md#4-event-types)

## Functionality

The Event Handler:

1. **Consumes** events from Redpanda topics (sensor-events, user-actions, system-events)
2. **Dispatches** events to appropriate projection handlers based on event type
3. **Updates** projections table (materialized views for queries)
4. **Tracks** consumer offset for at-least-once delivery
5. **Handles failures** by logging errors (Phase 1: no DLQ)

## Design

### Component Location

```
internal/services/eventhandler/
├── consumer.go       # Kafka consumer, event dispatch
├── handlers.go       # Projection update handlers
├── repository.go     # Interface definitions (ports)
└── migrations/       # Already exists (projections, dlq tables)
```

### Architecture: Consumer + Handler Registry

```
┌─────────────────────────────────────────────────────────────────┐
│                        Event Handler                            │
│                                                                 │
│  ┌────────────────────┐         ┌─────────────────────────┐    │
│  │   Kafka Consumer   │────────▶│    Handler Registry     │    │
│  │                    │         └───────────┬─────────────┘    │
│  │ - Subscribe topics │                     │                  │
│  │ - Poll messages    │         ┌───────────┼───────────┐      │
│  │ - Commit offsets   │         ▼           ▼           ▼      │
│  └────────────────────┘    [sensor.*]  [user.*]    [system.*]  │
│                                 │           │           │      │
│                                 └───────────┴───────────┘      │
│                                          │                     │
│                                          ▼                     │
│                              ┌─────────────────────┐           │
│                              │ ProjectionRepo      │           │
│                              │ - Upsert projection │           │
│                              └─────────────────────┘           │
└─────────────────────────────────────────────────────────────────┘
```

### Interfaces (Ports)

```go
// EventHandler processes events and updates projections
type EventHandler interface {
    Handle(ctx context.Context, event *events.Envelope) error
}

// ProjectionRepository manages projection state
type ProjectionRepository interface {
    Upsert(ctx context.Context, projectionType, aggregateID string, state any, event *events.Envelope) error
    Get(ctx context.Context, projectionType, aggregateID string) (*Projection, error)
}

// EventConsumer consumes events from the message bus
type EventConsumer interface {
    Subscribe(ctx context.Context, topics []string) error
    Poll(ctx context.Context) (*events.Envelope, error)
    Commit(ctx context.Context) error
    Close() error
}
```

### Handler Registry Pattern

```go
type HandlerRegistry struct {
    handlers map[string]EventHandler // event_type prefix -> handler
}

func (r *HandlerRegistry) Register(prefix string, handler EventHandler) {
    r.handlers[prefix] = handler
}

func (r *HandlerRegistry) Dispatch(ctx context.Context, event *events.Envelope) error {
    for prefix, handler := range r.handlers {
        if strings.HasPrefix(event.EventType, prefix) {
            return handler.Handle(ctx, event)
        }
    }
    // No handler registered - log and skip (not an error)
    return nil
}
```

### Projection Update Logic

For Phase 1, a simple projection that stores the latest event payload per aggregate:

```go
type SensorProjectionHandler struct {
    repo ProjectionRepository
}

func (h *SensorProjectionHandler) Handle(ctx context.Context, event *events.Envelope) error {
    // For sensor.reading events, update the sensor's current state
    return h.repo.Upsert(ctx, "sensor_state", event.AggregateID, event.Payload, event)
}
```

The `projections` table stores:
- `projection_type`: e.g., "sensor_state", "user_session"
- `aggregate_id`: e.g., "device-001"
- `state`: JSONB with current projection data
- `last_event_id`: for idempotency checking
- `last_event_timestamp`: for ordering

### Idempotency

**Decision:** Check `last_event_id` before updating projection.

```go
func (r *ProjectionRepo) Upsert(ctx context.Context, projType, aggID string, state any, event *events.Envelope) error {
    query := `
        INSERT INTO projections (projection_type, aggregate_id, state, last_event_id, last_event_timestamp)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (projection_type, aggregate_id) DO UPDATE
        SET state = $3, last_event_id = $4, last_event_timestamp = $5, updated_at = NOW()
        WHERE projections.last_event_timestamp < $5
           OR (projections.last_event_timestamp = $5 AND projections.last_event_id < $4)
    `
    // Only update if event is newer (timestamp, then event_id as tiebreaker)
}
```

### Consumer Group and Offset Management

```go
// Consumer configuration
topics := []string{"sensor-events", "user-actions", "system-events"}
groupID := "event-handler"

// franz-go handles offset commits automatically when using consumer groups
// Manual commit after successful processing for at-least-once delivery
```

### Error Handling

**Phase 1 approach:** Log errors, continue processing. No DLQ.

| Scenario | Behavior |
|----------|----------|
| Projection update fails | Log ERROR, skip event, continue |
| Deserialization fails | Log ERROR, skip event, continue |
| Unknown event type | Log DEBUG, skip event (not an error) |
| Consumer connection fails | Reconnect with backoff |

**Rationale:** Phase 1 prioritizes simplicity. DLQ adds complexity. If projection updates fail consistently, logs will show the problem. Events are still in Redpanda for replay.

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `EVENTHANDLER_CONSUMER_GROUP` | event-handler | Kafka consumer group ID |
| `EVENTHANDLER_TOPICS` | sensor-events,user-actions,system-events | Comma-separated topics |
| `EVENTHANDLER_POLL_TIMEOUT` | 1s | Poll timeout |

## Files to Create/Modify

### New Files

- `internal/services/eventhandler/consumer.go` — Kafka consumer wrapper
- `internal/services/eventhandler/handlers.go` — Projection handlers
- `internal/services/eventhandler/repository.go` — Interface definitions
- `internal/shared/infra/postgres/projections.go` — ProjectionRepository implementation
- `internal/shared/infra/redpanda/consumer.go` — EventConsumer implementation

### Modify

- `cmd/platform/main.go` — Wire and start the event handler
- `internal/shared/config/config.go` — Add event handler config

## Acceptance Criteria

- [ ] Consumer subscribes to Redpanda topics on startup
- [ ] Events are consumed and dispatched to handlers
- [ ] Projections table is updated with event data
- [ ] Duplicate events are handled idempotently (no duplicate updates)
- [ ] Consumer offset is committed after successful processing
- [ ] Graceful shutdown completes in-flight processing
- [ ] Unknown event types are logged and skipped (not errors)

## Design Decisions

### Consumer Strategy

**Decision:** Single consumer subscribing to all topics (Phase 1).

See [ADR-0014](../../platform-docs/decisions/0014-event-handler-consumer-strategy.md) for rationale and future migration to topic-specific consumers with partition-based parallelism.

### Offset Commit Strategy

**Decision:** Manual commit after successful projection update.

This provides at-least-once delivery. If the consumer crashes after processing but before commit, the event will be reprocessed — handled by idempotency in projection updates.

## Notes

- The `projections` and `dlq` tables already exist in migrations
- franz-go (already in go.mod) supports consumer groups
- Event Handler uses `EVENTHANDLER_DATABASE_URL` per ADR-0010 (database-per-service)
- Topics match what Outbox Processor publishes to (sensor-events, user-actions, system-events)
