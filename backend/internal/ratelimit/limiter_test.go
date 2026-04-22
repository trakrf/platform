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

func TestLimiter_FreshKeyReportsLimitAsRemaining(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := newTestLimiter(clock)
	defer lim.Close()

	d := lim.Allow("key-a")

	require.True(t, d.Allowed, "first request on fresh key must be allowed")
	require.Equal(t, 60, d.Limit, "Limit reports steady-state rate per minute")
	require.Equal(t, 60, d.Remaining, "burst tokens above Limit are hidden; Remaining caps at Limit")
	require.Zero(t, d.RetryAfter, "allowed requests have no RetryAfter")
}

func TestLimiter_RemainingDecrementsOnceBelowLimit(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := newTestLimiter(clock)
	defer lim.Close()

	// Consume through the burst overflow (tokens > Limit): Remaining stays
	// pinned at Limit because the cap hides the burst safety margin.
	for i := 0; i < 60; i++ {
		d := lim.Allow("key-a")
		require.Equal(t, 60, d.Remaining,
			"request %d: tokens still ≥ Limit, Remaining pinned at Limit", i+1)
	}

	// Next request drops tokens to 59 (< Limit=60) — Remaining now tracks the
	// true token count.
	d := lim.Allow("key-a")
	require.Equal(t, 59, d.Remaining, "tokens below Limit: Remaining reflects actual bucket")

	d = lim.Allow("key-a")
	require.Equal(t, 58, d.Remaining)
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

func TestLimiter_KeysAreIndependent(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := newTestLimiter(clock)
	defer lim.Close()

	// Exhaust key-a.
	for i := 0; i < 120; i++ {
		lim.Allow("key-a")
	}
	require.False(t, lim.Allow("key-a").Allowed)

	// key-b untouched — should still have full burst under the hood, but
	// Remaining reports the advertised Limit ceiling.
	d := lim.Allow("key-b")
	require.True(t, d.Allowed)
	require.Equal(t, 60, d.Remaining)
}

func TestLimiter_SweepEvictsIdleBuckets(t *testing.T) {
	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)
	lim := NewLimiter(Config{
		RatePerMinute: 60,
		Burst:         120,
		IdleTTL:       1 * time.Hour,
		SweepInterval: 24 * time.Hour, // sweeper loop idle; tests drive sweep() directly
		Clock:         clock,
	})
	defer lim.Close()

	lim.Allow("key-a")
	lim.Allow("key-b")

	// Before cutoff — both present.
	clock.Advance(30 * time.Minute)
	lim.sweep()
	_, aPresent := lim.buckets.Load("key-a")
	_, bPresent := lim.buckets.Load("key-b")
	require.True(t, aPresent)
	require.True(t, bPresent)

	// Bump key-a's lastSeen at t = start + 30min.
	lim.Allow("key-a")

	// Advance 31 more minutes → t = start + 61min.
	// Cutoff = t - 60min = start + 1min.
	//   key-a lastSeen = start + 30min, > cutoff → survives.
	//   key-b lastSeen = start,          < cutoff → evicted.
	clock.Advance(31 * time.Minute)
	lim.sweep()

	_, aPresent = lim.buckets.Load("key-a")
	_, bPresent = lim.buckets.Load("key-b")
	require.True(t, aPresent, "key-a was touched recently, must survive")
	require.False(t, bPresent, "key-b idle >1h, must be evicted")
}

func TestLimiter_RemainingNeverExceedsLimit(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := newTestLimiter(clock)
	defer lim.Close()

	// Drive every possible bucket state: fresh full-burst, draining through burst
	// overflow, down to exhaustion. Limit is the externally-advertised ceiling —
	// clients pace against it and treat it as the maximum of Remaining. The
	// internal burst capacity is a safety margin, not advertised quota.
	for i := 0; i < 125; i++ {
		d := lim.Allow("key-a")
		require.LessOrEqualf(t, d.Remaining, d.Limit,
			"request %d: Remaining=%d must be ≤ Limit=%d", i+1, d.Remaining, d.Limit)
	}
}

func TestLimiter_ResetAtIsNowWhenBucketAboveLimit(t *testing.T) {
	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)
	lim := newTestLimiter(clock)
	defer lim.Close()

	// Fresh bucket has 120 tokens (burst), Limit=60. Since tokens ≥ Limit, the
	// client already has their full advertised quota — ResetAt should be "now"
	// (no wait required) rather than "time when burst refills".
	d := lim.Allow("key-a")
	require.Equal(t, start, d.ResetAt,
		"tokens above limit: ResetAt must equal now; remaining already at limit")
}

func TestLimiter_ResetAtReflectsTimeToRefillToLimit(t *testing.T) {
	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)
	lim := newTestLimiter(clock)
	defer lim.Close()

	// Drain fully. Bucket at 0 tokens, limit=60, refill rate 1/sec.
	// "Quota reset" = when Remaining next reaches Limit = 60 more tokens = 60s.
	for i := 0; i < 120; i++ {
		lim.Allow("key-a")
	}
	d := lim.Allow("key-a") // denied, bucket at 0

	expected := start.Add(60 * time.Second)
	require.WithinDuration(t, expected, d.ResetAt, 100*time.Millisecond,
		"bucket drained: ResetAt should be now + Limit/perSec (time to refill to Limit)")
}

func TestLimiter_SweptKeyReturnsWithFullBucket(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := NewLimiter(Config{
		RatePerMinute: 60,
		Burst:         120,
		IdleTTL:       1 * time.Hour,
		SweepInterval: 24 * time.Hour,
		Clock:         clock,
	})
	defer lim.Close()

	// Drain key-a's burst.
	for i := 0; i < 120; i++ {
		lim.Allow("key-a")
	}
	require.False(t, lim.Allow("key-a").Allowed)

	// Idle past TTL, sweep, return.
	clock.Advance(2 * time.Hour)
	lim.sweep()

	d := lim.Allow("key-a")
	require.True(t, d.Allowed)
	require.Equal(t, 60, d.Remaining, "fresh bucket after eviction")
}
