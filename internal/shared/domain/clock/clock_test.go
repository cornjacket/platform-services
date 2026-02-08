package clock

import (
	"testing"
	"time"
)

func TestRealClock_Now(t *testing.T) {
	before := time.Now().UTC()
	got := RealClock{}.Now()
	after := time.Now().UTC()

	if got.Before(before) || got.After(after) {
		t.Errorf("RealClock.Now() = %v, want between %v and %v", got, before, after)
	}
}

func TestFixedClock_Now(t *testing.T) {
	fixedTime := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	clock := FixedClock{Time: fixedTime}

	got := clock.Now()

	if !got.Equal(fixedTime) {
		t.Errorf("FixedClock.Now() = %v, want %v", got, fixedTime)
	}

	// Should return same time on multiple calls
	got2 := clock.Now()
	if !got2.Equal(fixedTime) {
		t.Errorf("FixedClock.Now() second call = %v, want %v", got2, fixedTime)
	}
}

func TestReplayClock_Advance(t *testing.T) {
	clock := &ReplayClock{}

	// Initial time should be zero
	if !clock.Now().IsZero() {
		t.Errorf("ReplayClock initial Now() = %v, want zero time", clock.Now())
	}

	// Advance to first event time
	event1Time := time.Date(2026, 2, 7, 10, 0, 0, 0, time.UTC)
	clock.Advance(event1Time)

	if !clock.Now().Equal(event1Time) {
		t.Errorf("ReplayClock.Now() after first advance = %v, want %v", clock.Now(), event1Time)
	}

	// Advance to second event time
	event2Time := time.Date(2026, 2, 7, 10, 5, 0, 0, time.UTC)
	clock.Advance(event2Time)

	if !clock.Now().Equal(event2Time) {
		t.Errorf("ReplayClock.Now() after second advance = %v, want %v", clock.Now(), event2Time)
	}
}

func TestPackageLevelClock(t *testing.T) {
	// Reset to real clock after test
	t.Cleanup(Reset)

	// Default should be real clock (close to current time)
	before := time.Now().UTC()
	got := Now()
	after := time.Now().UTC()

	if got.Before(before.Add(-time.Second)) || got.After(after.Add(time.Second)) {
		t.Errorf("Now() with default clock = %v, want close to current time", got)
	}

	// Set to fixed clock
	fixedTime := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	Set(FixedClock{Time: fixedTime})

	got = Now()
	if !got.Equal(fixedTime) {
		t.Errorf("Now() with fixed clock = %v, want %v", got, fixedTime)
	}

	// Reset should restore real clock
	Reset()
	got = Now()
	if got.Equal(fixedTime) {
		t.Errorf("Now() after Reset should not equal fixed time")
	}
}
