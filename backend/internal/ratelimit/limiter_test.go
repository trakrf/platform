package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestLimiter(clock Clock) *Limiter {
	return NewLimiter(Config{
		RatePerMinute: 60,
		Burst:         120,
		IdleTTL:       time.Hour,
		SweepInterval: 24 * time.Hour, // effectively disabled; tests call sweep() directly
		Clock:         clock,
	})
}

func TestLimiter_FreshKeyAllowedWithFullBurstMinusOne(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := newTestLimiter(clock)
	defer lim.Close()

	d := lim.Allow("key-a")

	require.True(t, d.Allowed, "first request on fresh key must be allowed")
	require.Equal(t, 60, d.Limit, "Limit reports steady-state rate per minute")
	require.Equal(t, 119, d.Remaining, "burst=120, one token consumed")
	require.Zero(t, d.RetryAfter, "allowed requests have no RetryAfter")
}

func TestLimiter_SuccessiveAllowsDecrementRemaining(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := newTestLimiter(clock)
	defer lim.Close()

	d1 := lim.Allow("key-a")
	d2 := lim.Allow("key-a")
	d3 := lim.Allow("key-a")

	require.True(t, d1.Allowed)
	require.True(t, d2.Allowed)
	require.True(t, d3.Allowed)
	require.Equal(t, 119, d1.Remaining)
	require.Equal(t, 118, d2.Remaining)
	require.Equal(t, 117, d3.Remaining)
}

func TestLimiter_ExhaustedBurstIsDenied(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := newTestLimiter(clock)
	defer lim.Close()

	// Drain the full burst.
	for i := 0; i < 120; i++ {
		d := lim.Allow("key-a")
		require.Truef(t, d.Allowed, "request %d should be allowed within burst", i+1)
	}

	// Next request — bucket empty, no time passed, expect denial.
	d := lim.Allow("key-a")

	require.False(t, d.Allowed, "121st request must be denied")
	require.Equal(t, 0, d.Remaining, "Remaining floored at 0")
	require.Equal(t, time.Second, d.RetryAfter, "at 1 token/sec, next token arrives in ~1s (ceiled to 1s)")
}

func TestLimiter_RefillsOverTime(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := newTestLimiter(clock)
	defer lim.Close()

	// Exhaust burst.
	for i := 0; i < 120; i++ {
		lim.Allow("key-a")
	}
	require.False(t, lim.Allow("key-a").Allowed)

	// Advance 1 second — at 60/min (= 1/sec), one token refills.
	clock.Advance(1 * time.Second)
	d := lim.Allow("key-a")
	require.True(t, d.Allowed, "after 1s refill, one request allowed")
	require.Equal(t, 0, d.Remaining, "that one refilled token was consumed")

	// Advance 2 more seconds — two more tokens refill.
	clock.Advance(2 * time.Second)
	require.True(t, lim.Allow("key-a").Allowed)
	require.True(t, lim.Allow("key-a").Allowed)
	require.False(t, lim.Allow("key-a").Allowed, "third in the same instant is denied")
}
