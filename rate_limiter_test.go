package slog

import (
	"testing"
	"time"
)

type manualClock struct {
	now time.Time
}

func (c *manualClock) Now() time.Time { return c.now }

func (c *manualClock) Advance(duration time.Duration) { c.now = c.now.Add(duration) }

func TestRateLimiterBurstRefillAndCapacity(t *testing.T) {
	clock := &manualClock{now: time.Unix(1, 0)}
	limiter := newRateLimiterWithClock(2, 2, clock.Now)

	if !limiter.Allow() || !limiter.Allow() {
		t.Fatal("initial burst should allow two records")
	}
	if limiter.Allow() {
		t.Fatal("record beyond burst was allowed")
	}

	clock.Advance(250 * time.Millisecond)
	if limiter.Allow() {
		t.Fatal("half a token should not allow a record")
	}
	clock.Advance(250 * time.Millisecond)
	if !limiter.Allow() {
		t.Fatal("one refilled token should allow a record")
	}

	clock.Advance(time.Hour)
	if !limiter.Allow() || !limiter.Allow() {
		t.Fatal("long refill should restore the full burst")
	}
	if limiter.Allow() {
		t.Fatal("refill exceeded configured burst capacity")
	}
}

func TestRateLimiterDefaultsBurstToRate(t *testing.T) {
	clock := &manualClock{now: time.Unix(1, 0)}
	limiter := newRateLimiterWithClock(3, 0, clock.Now)
	for i := 0; i < 3; i++ {
		if !limiter.Allow() {
			t.Fatalf("Allow() call %d = false, want default burst of 3", i+1)
		}
	}
	if limiter.Allow() {
		t.Fatal("fourth record exceeded the default burst")
	}
}

func TestRateLimiterConfigureResetsBucketAndCanDisable(t *testing.T) {
	clock := &manualClock{now: time.Unix(1, 0)}
	limiter := newRateLimiterWithClock(1, 1, clock.Now)
	if !limiter.Allow() || limiter.Allow() {
		t.Fatal("unexpected initial bucket behavior")
	}

	limiter.configure(4, 1, true)
	if !limiter.Allow() || limiter.Allow() {
		t.Fatal("configure should reset the bucket to the new burst")
	}
	clock.Advance(250 * time.Millisecond)
	if !limiter.Allow() {
		t.Fatal("configured refill rate did not produce a token")
	}

	limiter.configure(0, 0, false)
	for i := 0; i < 10; i++ {
		if !limiter.Allow() {
			t.Fatal("disabled limiter rejected a record")
		}
	}
}

func TestRateLimiterNilAndNonPositiveRateAllowAll(t *testing.T) {
	var nilLimiter *rateLimiter
	if !nilLimiter.Allow() {
		t.Fatal("nil limiter should be disabled")
	}

	limiter := newRateLimiterWithClock(0, 10, nil)
	if !limiter.Allow() {
		t.Fatal("non-positive rate should disable limiting")
	}
}
