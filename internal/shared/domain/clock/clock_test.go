package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRealClock_Now(t *testing.T) {
	before := time.Now().UTC()
	got := RealClock{}.Now()
	after := time.Now().UTC()

	assert.False(t, got.Before(before), "should not be before current time")
	assert.False(t, got.After(after), "should not be after current time")
}

func TestFixedClock_Now(t *testing.T) {
	fixedTime := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	clock := FixedClock{Time: fixedTime}

	assert.Equal(t, fixedTime, clock.Now())
	assert.Equal(t, fixedTime, clock.Now(), "should return same time on multiple calls")
}

func TestReplayClock_Advance(t *testing.T) {
	clock := &ReplayClock{}
	assert.True(t, clock.Now().IsZero(), "initial time should be zero")

	event1Time := time.Date(2026, 2, 7, 10, 0, 0, 0, time.UTC)
	clock.Advance(event1Time)
	assert.Equal(t, event1Time, clock.Now())

	event2Time := time.Date(2026, 2, 7, 10, 5, 0, 0, time.UTC)
	clock.Advance(event2Time)
	assert.Equal(t, event2Time, clock.Now())
}

func TestPackageLevelClock(t *testing.T) {
	t.Cleanup(Reset)

	// Default should be real clock
	before := time.Now().UTC()
	got := Now()
	after := time.Now().UTC()
	assert.False(t, got.Before(before.Add(-time.Second)))
	assert.False(t, got.After(after.Add(time.Second)))

	// Set to fixed clock
	fixedTime := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	Set(FixedClock{Time: fixedTime})
	assert.Equal(t, fixedTime, Now())

	// Reset should restore real clock
	Reset()
	assert.NotEqual(t, fixedTime, Now())
}
