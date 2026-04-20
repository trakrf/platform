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
