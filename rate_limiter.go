package slog

import (
	"sync"
	"time"
)

type rateLimiter struct {
	mu         sync.Mutex
	capacity   int
	tokens     float64
	refill     float64
	lastRefill time.Time
	now        func() time.Time
	enabled    bool
}

func newRateLimiter(ratePerSecond, burst int) *rateLimiter {
	return newRateLimiterWithClock(ratePerSecond, burst, time.Now)
}

func newRateLimiterWithClock(ratePerSecond, burst int, now func() time.Time) *rateLimiter {
	if now == nil {
		now = time.Now
	}
	if ratePerSecond <= 0 {
		return &rateLimiter{enabled: false, now: now}
	}
	if burst <= 0 {
		burst = ratePerSecond
	}
	return &rateLimiter{
		capacity:   burst,
		tokens:     float64(burst),
		refill:     float64(ratePerSecond),
		lastRefill: now(),
		now:        now,
		enabled:    true,
	}
}

func (rl *rateLimiter) Allow() bool {
	if rl == nil || !rl.enabled {
		return true
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	if elapsed > 0 {
		rl.tokens += elapsed * rl.refill
		if rl.tokens > float64(rl.capacity) {
			rl.tokens = float64(rl.capacity)
		}
		rl.lastRefill = now
	}
	if rl.tokens < 1 {
		return false
	}
	rl.tokens -= 1
	return true
}

func (rl *rateLimiter) configure(ratePerSecond, burst int, enabled bool) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.enabled = enabled && ratePerSecond > 0
	if !rl.enabled {
		return
	}
	if burst <= 0 {
		burst = ratePerSecond
	}
	rl.capacity = burst
	rl.tokens = float64(burst)
	rl.refill = float64(ratePerSecond)
	rl.lastRefill = rl.now()
}
