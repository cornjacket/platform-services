# Task 008: Time Handling Strategy

**Status:** Draft
**Created:** 2026-02-07
**Updated:** 2026-02-07

## Context

Currently, `NewEnvelope()` calls `time.Now().UTC()` directly, creating two problems:

1. **Testability** — Cannot assert exact timestamps in unit tests
2. **Domain accuracy** — IoT devices know when events occurred; we discard that information

Additionally, event-driven systems need to distinguish between:
- **Event time** — when the event actually happened (at the source)
- **Ingestion time** — when the platform received it

## Goals

1. Introduce dual timestamps: `EventTime` and `IngestedAt`
2. Create a `clock` package for time abstraction
3. Enable deterministic unit tests
4. Support future replay scenarios

## Design

### Dual Timestamps

| Timestamp | Who Sets It | Purpose |
|-----------|-------------|---------|
| `EventTime` | Caller/Producer | Business logic, analytics — "when did this happen?" |
| `IngestedAt` | Platform (via clock) | Auditing, SLA tracking — "when did we receive it?" |

**Real-world scenario:**
```
Device buffers 3 readings during network outage:
  Reading 1: EventTime = 10:00:00  (temperature spike)
  Reading 2: EventTime = 10:01:00  (normal)
  Reading 3: EventTime = 10:02:00  (normal)

Network recovers, device sends batch at 10:15:00:
  All 3 arrive with IngestedAt = 10:15:00

Without EventTime, you'd think all readings happened at 10:15.
Without IngestedAt, you can't track ingestion latency.
```

### Clock Package

Package-level clock avoids threading clock through every function signature:

```go
// internal/shared/domain/clock/clock.go
package clock

import (
    "sync"
    "time"
)

type Clock interface {
    Now() time.Time
}

var (
    mu      sync.RWMutex
    current Clock = RealClock{}
)

// Now returns the current time from the active clock
func Now() time.Time {
    mu.RLock()
    defer mu.RUnlock()
    return current.Now()
}

// Set replaces the active clock (for testing/replay)
func Set(c Clock) {
    mu.Lock()
    defer mu.Unlock()
    current = c
}

// Reset restores the real clock
func Reset() {
    Set(RealClock{})
}

// RealClock uses actual system time
type RealClock struct{}

func (RealClock) Now() time.Time {
    return time.Now().UTC()
}

// FixedClock returns a predetermined time (for testing)
type FixedClock struct {
    Time time.Time
}

func (c FixedClock) Now() time.Time {
    return c.Time
}

// ReplayClock advances time based on events being replayed
type ReplayClock struct {
    mu   sync.Mutex
    time time.Time
}

func (c *ReplayClock) Now() time.Time {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.time
}

func (c *ReplayClock) Advance(t time.Time) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.time = t
}
```

### Updated Envelope

```go
type Envelope struct {
    EventID     uuid.UUID       `json:"event_id"`
    EventType   string          `json:"event_type"`
    AggregateID string          `json:"aggregate_id"`
    EventTime   time.Time       `json:"event_time"`    // when it happened (from caller)
    IngestedAt  time.Time       `json:"ingested_at"`   // when platform received it
    Payload     json.RawMessage `json:"payload"`
    Metadata    Metadata        `json:"metadata"`
}

// NewEnvelope creates an event envelope.
// eventTime is provided by the caller; IngestedAt is set by the platform clock.
func NewEnvelope(
    eventType, aggregateID string,
    payload any,
    metadata Metadata,
    eventTime time.Time,
) (*Envelope, error) {
    payloadBytes, err := json.Marshal(payload)
    if err != nil {
        return nil, err
    }

    return &Envelope{
        EventID:     uuid.Must(uuid.NewV7()),
        EventType:   eventType,
        AggregateID: aggregateID,
        EventTime:   eventTime,
        IngestedAt:  clock.Now(),
        Payload:     payloadBytes,
        Metadata:    metadata,
    }, nil
}
```

### Usage Patterns

**Production:**
```go
// Default clock is RealClock — no setup needed
envelope := events.NewEnvelope(
    "sensor.reading",
    "device-1",
    payload,
    metadata,
    deviceTime,  // from request
)
// envelope.IngestedAt = actual time.Now().UTC()
```

**Unit tests:**
```go
func TestEnvelope_IngestedAt(t *testing.T) {
    fixedTime := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
    clock.Set(clock.FixedClock{Time: fixedTime})
    t.Cleanup(clock.Reset)

    eventTime := time.Date(2026, 2, 7, 10, 0, 0, 0, time.UTC)
    envelope, _ := events.NewEnvelope("sensor.reading", "device-1", payload, meta, eventTime)

    assert.Equal(t, eventTime, envelope.EventTime)
    assert.Equal(t, fixedTime, envelope.IngestedAt)  // deterministic!
}
```

**Replay:**
```go
replayClock := &clock.ReplayClock{}
clock.Set(replayClock)
defer clock.Reset()

for _, historicalEvent := range events {
    replayClock.Advance(historicalEvent.IngestedAt)
    process(historicalEvent)  // sees original IngestedAt
}
```

### API Changes

The ingestion API will accept an optional `event_time` field:

```json
{
  "event_type": "sensor.reading",
  "aggregate_id": "device-001",
  "payload": {"value": 72.5, "unit": "fahrenheit"},
  "event_time": "2026-02-07T10:00:00Z"
}
```

If `event_time` is omitted, the platform uses `clock.Now()` as a fallback.

## Files to Create/Modify

### Create

| File | Description |
|------|-------------|
| `internal/shared/domain/clock/clock.go` | Clock interface, RealClock, FixedClock, ReplayClock |

### Modify

| File | Changes |
|------|---------|
| `internal/shared/domain/events/envelope.go` | Add `EventTime`, rename `Timestamp` to `IngestedAt`, update `NewEnvelope()` signature |
| `internal/services/ingestion/handler.go` | Parse `event_time` from request, pass to service |
| `internal/services/ingestion/service.go` | Pass `eventTime` to `NewEnvelope()` |
| `internal/shared/infra/postgres/outbox.go` | Handle new envelope schema |
| `internal/shared/infra/postgres/eventstore.go` | Handle new envelope schema |
| `internal/services/eventhandler/handlers.go` | Handle new envelope schema |
| `api/openapi/ingestion.yaml` | Add `event_time` field to request schema |

### Database Migrations

| File | Changes |
|------|---------|
| `internal/services/ingestion/migrations/003_rename_timestamp.sql` | Rename `timestamp` → `event_time`, add `ingested_at` |
| `internal/services/eventhandler/migrations/003_update_projections.sql` | Update schema if needed |

## Subtasks

### Documentation Updates

- [ ] **ADR-0015: Time Handling Strategy** — Document the architectural decision for dual timestamps and clock injection
- [ ] **design-spec.md** — Add section on time handling (event time vs ingestion time, clock abstraction)
- [ ] **ARCHITECTURE.md** — Add `clock` package to layer mapping (Domain layer)
- [ ] **DEVELOPMENT.md** — Update project structure to show `clock/` package
- [ ] **OpenAPI spec** — Document `event_time` field in ingestion request

### Implementation

- [ ] Create `clock` package with `RealClock`, `FixedClock`, `ReplayClock`
- [ ] Update `Envelope` struct with dual timestamps
- [ ] Update `NewEnvelope()` signature
- [ ] Update all callers of `NewEnvelope()`
- [ ] Update ingestion handler to parse `event_time`
- [ ] Create database migration for schema change
- [ ] Update existing tests

### Testing

- [ ] Unit test `clock` package
- [ ] Unit test `NewEnvelope()` with fixed clock
- [ ] Verify E2E tests pass with new schema

## Acceptance Criteria

- [ ] `clock.Now()` is used instead of `time.Now()` throughout codebase
- [ ] `Envelope` has both `EventTime` and `IngestedAt` fields
- [ ] Unit tests can assert exact timestamps
- [ ] API accepts optional `event_time` field
- [ ] Database schema updated with migration
- [ ] All existing E2E tests pass
- [ ] ADR-0015 documents the decision
- [ ] Documentation updated

## Notes

### Industry Precedent

| System | Pattern |
|--------|---------|
| **Kafka** | `CreateTime` (producer) vs `LogAppendTime` (broker) |
| **CloudEvents** | Single `time` field (event time) |
| **Event Store** | `Created` (producer) vs `Recorded` (server) |
| **Apache Flink** | Event time vs Processing time vs Ingestion time |

### Design Principles

1. **EventTime** comes from the caller — only they know when it happened
2. **IngestedAt** is set by the platform — it's the platform's audit record
3. Callers should NOT be able to set `IngestedAt` (security/audit concern)
4. Clock injection enables testing and replay without changing the API

### Future Considerations

- **ReplayClock** enables exact replay of historical events
- Clock abstraction could support accelerated time for load testing
- Consider adding `ProcessedAt` for event handler processing time
