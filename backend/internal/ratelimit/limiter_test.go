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

func TestLimiter_FreshKeyAdvertisesBurstCeiling(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := newTestLimiter(clock)
	defer lim.Close()

	d := lim.Allow("key-a")

	require.True(t, d.Allowed, "first request on fresh key must be allowed")
	require.Equal(t, 120, d.Limit, "Limit advertises the true ceiling (burst) — the max requests before a 429")
	require.Equal(t, 119, d.Remaining, "one of 120 burst tokens consumed; Remaining counts down 1:1 from the first request")
	require.Zero(t, d.RetryAfter, "allowed requests have no RetryAfter")
	require.Equal(t, 60, d.PolicyQuota, "sustained quota is RatePerMinute")
	require.Equal(t, 60, d.PolicyWindowSec, "sustained window is 60s")
}

func TestLimiter_RemainingDecrementsOneToOneFromFirstRequest(t *testing.T) {
	clock := NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := newTestLimiter(clock)
	defer lim.Close()

	// No flat zone (the pre-Option-A burst-margin masking is gone): every
	// request drops Remaining by exactly 1, from 119 down. This is the POLS
	// contract a generic SDK assumes (TRA-878).
	for i := 0; i < 120; i++ {
		d := lim.Allow("key-a")
		require.Equalf(t, 119-i, d.Remaining,
			"request %d: Remaining must be %d (1:1 countdown, no burst-margin masking)", i+1, 119-i)
	}
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

	// key-b untouched — its own full burst; counts down from 119.
	d := lim.Allow("key-b")
	require.True(t, d.Allowed)
	require.Equal(t, 119, d.Remaining)
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

	// Drive every bucket state: fresh full-burst down to exhaustion. Limit is
	// the advertised ceiling (the burst); Remaining is the live token count and
	// must never exceed it.
	for i := 0; i < 125; i++ {
		d := lim.Allow("key-a")
		require.LessOrEqualf(t, d.Remaining, d.Limit,
			"request %d: Remaining=%d must be ≤ Limit=%d", i+1, d.Remaining, d.Limit)
		require.GreaterOrEqual(t, d.Remaining, 0, "Remaining floored at 0")
	}
}

func TestLimiter_AnonDecisionAdvertisesFullCeiling(t *testing.T) {
	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)
	lim := newTestLimiter(clock)
	defer lim.Close()

	// The pre-auth / unrecognized-caller decision: untouched full bucket, so
	// Remaining == Limit == ceiling and ResetAt is now (no wait).
	d := lim.AnonDecision()
	require.True(t, d.Allowed)
	require.Equal(t, 120, d.Limit)
	require.Equal(t, 120, d.Remaining)
	require.Equal(t, start, d.ResetAt)
	require.Equal(t, 60, d.PolicyQuota)
	require.Equal(t, 60, d.PolicyWindowSec)
}

func TestLimiter_ResetAtIsNowWhenBucketFull(t *testing.T) {
	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)
	lim := newTestLimiter(clock)
	defer lim.Close()

	// An untouched bucket sits at the ceiling, so "when Remaining next equals
	// Limit" is now — no wait required.
	d := lim.AnonDecision()
	require.Equal(t, start, d.ResetAt,
		"full bucket: ResetAt must equal now; Remaining already at the ceiling")
}

func TestLimiter_ResetAtReflectsTimeToRefillToCeiling(t *testing.T) {
	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)
	lim := newTestLimiter(clock)
	defer lim.Close()

	// Drain fully. Bucket at 0 tokens, ceiling=120, refill rate 1/sec.
	// "Quota reset" = when Remaining next reaches Limit = 120 more tokens = 120s.
	for i := 0; i < 120; i++ {
		lim.Allow("key-a")
	}
	d := lim.Allow("key-a") // denied, bucket at 0

	expected := start.Add(120 * time.Second)
	require.WithinDuration(t, expected, d.ResetAt, 100*time.Millisecond,
		"bucket drained: ResetAt should be now + ceiling/perSec (time to refill to the ceiling)")
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
	require.Equal(t, 119, d.Remaining, "fresh bucket after eviction, one token consumed")
}
