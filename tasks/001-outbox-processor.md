# Task 001: Outbox Processor

**Status:** Draft
**Created:** 2026-02-04
**Updated:** 2026-02-04

## Context

Phase 1 of the platform requires end-to-end event flow:

```
HTTP Request → Ingestion → Outbox → Event Store + Redpanda → Consumer
                            ▲
                            └── This task
```

The Ingestion Service writes events to the outbox table. The Outbox Processor reads from the outbox, writes to the event store, publishes to Redpanda, and deletes the processed entry.

See:
- [ADR-0002 Outbox-First Write Pattern](../../platform-docs/decisions/0002-outbox-first-write-pattern.md)
- [ADR-0012 Outbox Processing Strategy](../../platform-docs/decisions/0012-outbox-processing-strategy.md)

## Functionality

The Outbox Processor:

1. **Listens** for new outbox entries via PostgreSQL NOTIFY/LISTEN
2. **Fetches** pending entries from the outbox table
3. **Writes** each event to the event store (same database, different table)
4. **Publishes** each event to Redpanda (Kafka-compatible message bus)
5. **Deletes** the entry from the outbox after successful processing
6. **Handles failures** by incrementing retry count, logging errors

## Design

### Component Location

```
internal/services/outbox/
├── processor.go      # Main processor logic
├── repository.go     # Interface definitions (ports)
└── (no handler.go - this is a background worker, not HTTP)
```

### Architecture: Dispatcher + Worker Pool

Per ADR-0012, we use a dispatcher + worker pool pattern:

```
┌─────────────────────────────────────────────────────────────────┐
│                        Outbox Processor                         │
│                                                                 │
│  ┌────────────────────┐         ┌─────────────────────────┐    │
│  │     Dispatcher     │────────▶│      Work Channel       │    │
│  │                    │         └───────────┬─────────────┘    │
│  │ - LISTEN (NOTIFY)  │                     │                  │
│  │ - Watchdog timer   │         ┌───────────┼───────────┐      │
│  │ - Fetch batch      │         ▼           ▼           ▼      │
│  └────────────────────┘    [Worker 1]  [Worker 2]  [Worker N]  │
│           ▲                     │           │           │      │
│           │                     └───────────┴───────────┘      │
│   Reset timer on                        │                      │
│   successful NOTIFY                     ▼                      │
│                              ┌─────────────────────┐           │
│                              │ For each entry:     │           │
│                              │ 1. Write event store│           │
│                              │ 2. Publish Redpanda │           │
│                              │ 3. Delete outbox    │           │
│                              └─────────────────────┘           │
└─────────────────────────────────────────────────────────────────┘
```

### Watchdog Timer

NOTIFY/LISTEN can fail silently (connection drop, PG restart, app restart). The watchdog timer ensures reliability:

1. **NOTIFY received** → Process immediately, reset timer
2. **Timer fires** (no NOTIFY) → Poll outbox anyway
3. **Guarantees** eventual processing regardless of NOTIFY health

```go
func (d *Dispatcher) run(ctx context.Context) {
    timer := time.NewTimer(pollInterval)
    for {
        select {
        case <-notifyChan:
            timer.Reset(pollInterval)  // reset watchdog
            d.fetchAndDispatch(ctx)
        case <-timer.C:
            d.fetchAndDispatch(ctx)    // watchdog fired
            timer.Reset(pollInterval)
        case <-ctx.Done():
            return
        }
    }
}
```

### Interfaces (Ports)

```go
// OutboxReader reads and manages outbox entries
type OutboxReader interface {
    FetchPending(ctx context.Context, limit int) ([]OutboxEntry, error)
    Delete(ctx context.Context, outboxID string) error
    IncrementRetry(ctx context.Context, outboxID string) error
}

// EventStoreWriter writes events to the event store
type EventStoreWriter interface {
    Insert(ctx context.Context, event *events.Envelope) error
}

// EventPublisher publishes events to the message bus
type EventPublisher interface {
    Publish(ctx context.Context, event *events.Envelope) error
}
```

### NOTIFY/LISTEN Mechanism

PostgreSQL trigger notifies on outbox insert:

```sql
-- Add to ingestion migrations
CREATE OR REPLACE FUNCTION notify_outbox_insert()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM pg_notify('outbox_channel', NEW.outbox_id::text);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER outbox_insert_trigger
    AFTER INSERT ON outbox
    FOR EACH ROW EXECUTE FUNCTION notify_outbox_insert();
```

Go listener:

```go
func (p *Processor) listenForNotifications(ctx context.Context) {
    conn.Exec(ctx, "LISTEN outbox_channel")
    for {
        notification, _ := conn.WaitForNotification(ctx)
        p.processOutbox(ctx)
    }
}
```

### Processing Logic

```go
func (p *Processor) processOutbox(ctx context.Context) error {
    entries, _ := p.outbox.FetchPending(ctx, batchSize)

    for _, entry := range entries {
        // Write to event store
        if err := p.eventStore.Insert(ctx, entry.Payload); err != nil {
            p.outbox.IncrementRetry(ctx, entry.OutboxID)
            continue
        }

        // Publish to Redpanda
        if err := p.publisher.Publish(ctx, entry.Payload); err != nil {
            // Event is in store but not published - log and continue
            // Next run will skip (idempotent) or DLQ (future)
            continue
        }

        // Delete from outbox
        p.outbox.Delete(ctx, entry.OutboxID)
    }
}
```

### Error Handling

**Phase 1 approach:** Retry all errors, no DLQ.

| Scenario | Behavior |
|----------|----------|
| Event store write fails | Increment retry, log ERROR, continue to next entry |
| Redpanda publish fails | Increment retry, log ERROR, continue |
| Delete fails | Log ERROR, entry will be reprocessed (idempotent) |
| Max retries exceeded | Log ERROR, leave in outbox as evidence |

### Why No DLQ for Outbox Processor

**Decision:** DLQ adds complexity without clear benefit for this component.

**Reasoning:**

1. **Transient errors (DB down, network timeout):** If Postgres is down, writing to DLQ would also fail. The outbox itself is our durability guarantee — we retry until infrastructure recovers.

2. **Permanent errors are unlikely:** The Ingestion Service validates events before writing to outbox. Both outbox and event_store use the same envelope schema. If validation passes at ingestion, there's no reason for event_store write to fail permanently.

3. **Duplicate event_id:** Handled via idempotency (unique constraint), not DLQ. See Open Questions.

4. **Redpanda failures:** Topic-missing or message-too-large are deployment/config bugs, not data issues. Fix the config, retry succeeds.

**If we see real permanent failures in production:** Add DLQ then, with evidence of what failures occur.

### Alerting (Production Requirement)

**Not implemented in Phase 1, but required for production:**

- [ ] Max retries exceeded should trigger alert
- [ ] Outbox table growth (entries not draining) should trigger alarm

**Code Smell:** Until alerting is implemented, retry exhaustion will go unnoticed. This is acceptable for dev but **must be addressed before production**. Track in ARCHITECTURE.md as a known smell.

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTBOX_WORKER_COUNT` | 4 | Number of worker goroutines |
| `OUTBOX_BATCH_SIZE` | 100 | Max entries per fetch |
| `OUTBOX_MAX_RETRIES` | 5 | Max retry attempts |
| `OUTBOX_POLL_INTERVAL` | 5s | Watchdog timer interval |

## Files to Create/Modify

### New Files

- `internal/services/outbox/processor.go` — Main processor logic
- `internal/services/outbox/repository.go` — Interface definitions
- `internal/shared/infra/postgres/eventstore.go` — EventStoreWriter implementation
- `internal/shared/infra/redpanda/producer.go` — EventPublisher implementation
- `internal/services/ingestion/migrations/003_outbox_notify_trigger.sql` — NOTIFY trigger

### Modify

- `cmd/platform/main.go` — Wire and start the processor
- `internal/shared/infra/postgres/outbox.go` — Already has FetchPending, Delete, IncrementRetry

## Acceptance Criteria

- [ ] Processor starts and listens for NOTIFY events
- [ ] New outbox entries are processed within seconds
- [ ] Events appear in event_store table after processing
- [ ] Events are published to Redpanda topic
- [ ] Processed entries are deleted from outbox
- [ ] Failed entries have retry_count incremented
- [ ] Graceful shutdown waits for in-flight processing

## Open Questions

1. **Transaction boundary:** Should event store write + outbox delete be in one transaction?
   - Pro: Atomicity
   - Con: Couples two operations, Redpanda publish is outside transaction anyway

2. **Idempotency:** How to handle duplicate processing if delete fails?
   - Option A: Event store has unique constraint on event_id
   - Option B: Check before insert

3. **Redpanda topic selection:** One topic or per-event-type topics?
   - Design spec says per-event-type: `sensor-events`, `user-actions`, `system-events`

## Notes

- The `OutboxEntry` type already exists in `infra/postgres/outbox.go` — may need to move to domain or outbox service package
- NOTIFY/LISTEN requires a dedicated connection (not from pool)
- **Single-instance only:** This design does not support multiple processor instances (see ADR-0012 for scaling paths)
