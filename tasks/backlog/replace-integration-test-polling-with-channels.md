# Replace Integration Test Polling with Channel-Based Pattern

## Context

The integration test `TestConsumerRoundTrip` in `internal/services/eventhandler/consumer_integration_test.go` uses a polling loop with a mutex and 50ms sleep to wait for async results:

```go
var mu sync.Mutex
var received *events.Envelope

// ... produce event ...

deadline := time.After(5 * time.Second)
for {
    mu.Lock()
    got := received
    mu.Unlock()
    if got != nil {
        break
    }
    select {
    case <-deadline:
        t.Fatal("timed out")
    case <-time.After(50 * time.Millisecond):
        // poll again
    }
}
```

The component tests (Spec 012) introduced a channel-based mock pattern that is superior:

```go
select {
case call := <-mock.calls:
    assert.Equal(t, expected, call)
case <-time.After(5 * time.Second):
    t.Fatal("timed out")
}
```

## Proposed Change

Replace the polling/mutex pattern in `TestConsumerRoundTrip` with the channel-based pattern. This would:

- Remove the `sync.Mutex` and polling loop
- Use a channel-based `mockEventHandler` that writes to a buffered channel
- Block on channel receive with timeout (instant wake-up, no 50ms polling interval)

## Affected Files

- `internal/services/eventhandler/consumer_integration_test.go` — rewrite async assertion
- `internal/services/eventhandler/testhelpers_test.go` — may need a channel-based mock variant

## Notes

- This is a test-only change; no production code affected.
- The integration test tests a different boundary (individual consumer adapter) than the component test (full service pipeline via `Start()`), so both tests should continue to exist.
- See `platform-docs/design-spec.md` section 15.3 for documentation of the channel-based pattern.
