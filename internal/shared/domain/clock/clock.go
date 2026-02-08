// Package clock provides time abstraction for testability and replay.
//
// Instead of calling time.Now() directly, code should call clock.Now().
// This allows tests to inject fixed times and replay scenarios to
// advance time based on historical events.
//
// Usage:
//
//	// Production code (uses real time by default)
//	timestamp := clock.Now()
//
//	// Tests (inject fixed time)
//	clock.Set(clock.FixedClock{Time: fixedTime})
//	t.Cleanup(clock.Reset)
//
//	// Replay (advance time per event)
//	replayClock := &clock.ReplayClock{}
//	clock.Set(replayClock)
//	replayClock.Advance(event.IngestedAt)
package clock

import (
	"sync"
	"time"
)

// Clock provides the current time.
type Clock interface {
	Now() time.Time
}

// Package-level clock (default: real time)
var (
	mu      sync.RWMutex
	current Clock = RealClock{}
)

// Now returns the current time from the active clock.
func Now() time.Time {
	mu.RLock()
	defer mu.RUnlock()
	return current.Now()
}

// Set replaces the active clock. Use for testing or replay.
func Set(c Clock) {
	mu.Lock()
	defer mu.Unlock()
	current = c
}

// Reset restores the real clock. Call in test cleanup.
func Reset() {
	Set(RealClock{})
}

// RealClock uses the actual system time.
type RealClock struct{}

// Now returns the current UTC time.
func (RealClock) Now() time.Time {
	return time.Now().UTC()
}

// FixedClock returns a predetermined time. Useful for unit tests.
type FixedClock struct {
	Time time.Time
}

// Now returns the fixed time.
func (c FixedClock) Now() time.Time {
	return c.Time
}

// ReplayClock advances time based on events being replayed.
// Call Advance() before processing each historical event.
type ReplayClock struct {
	mu   sync.Mutex
	time time.Time
}

// Now returns the current replay time.
func (c *ReplayClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.time
}

// Advance sets the replay time to the given timestamp.
func (c *ReplayClock) Advance(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.time = t
}
