# TRA-395 — Rate Limiting Middleware Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement per-key token-bucket rate limiting middleware for API-key-authenticated public API requests, matching the already-published docs at `docs.preview.trakrf.id/docs/api/rate-limits/` exactly.

**Architecture:** Two packages. `backend/internal/ratelimit/` owns a pure-Go `Limiter` built on `golang.org/x/time/rate`, keyed by JWT `jti`, with a TTL sweeper for idle buckets. `backend/internal/middleware/ratelimit.go` is a thin HTTP adapter: pulls the API-key principal, calls `Limiter.Allow`, emits `X-RateLimit-*` headers, and writes a 429 on exhaustion. A clock abstraction makes the limiter deterministic under test.

**Tech Stack:** Go, `golang.org/x/time/rate` (token bucket), `sync.Map` + `atomic.Int64`, chi router, zerolog logger, testify `require`, `net/http/httptest`.

**Spec:** `docs/superpowers/specs/2026-04-20-tra-395-rate-limiting-design.md`

---

## File Structure

| Path | Responsibility |
|------|----------------|
| `backend/internal/ratelimit/clock.go` | `Clock` interface, `RealClock`, `FakeClock` |
| `backend/internal/ratelimit/limiter.go` | `Limiter` + `bucket`, `Config`, `DefaultConfig`, `Allow`, `Close`, `sweep` |
| `backend/internal/ratelimit/limiter_test.go` | Unit tests using `FakeClock` |
| `backend/internal/middleware/ratelimit.go` | HTTP adapter: `RateLimit(limiter)` middleware |
| `backend/internal/middleware/ratelimit_test.go` | httptest-level tests |
| `backend/go.mod` / `go.sum` | Add `golang.org/x/time/rate` |
| `backend/internal/cmd/serve/router.go` | Mount the middleware on the EitherAuth group |
| `docs/api/openapi.public.yaml` | Verify/add 429 + `X-RateLimit-*` header schemas |

---

## Task 1: Add `golang.org/x/time/rate` dependency

**Files:**
- Modify: `backend/go.mod`, `backend/go.sum`

- [ ] **Step 1: Add the dependency**

```bash
cd backend && go get golang.org/x/time/rate && cd ..
```

Expected: `go.mod` gains `golang.org/x/time vX.Y.Z` in `require`.

- [ ] **Step 2: Verify build**

```bash
just backend build
```

Expected: clean build, no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/go.mod backend/go.sum
git commit -m "chore(tra-395): add golang.org/x/time/rate dependency"
```

---

## Task 2: Clock abstraction

**Files:**
- Create: `backend/internal/ratelimit/clock.go`
- Test: `backend/internal/ratelimit/clock_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/ratelimit/clock_test.go`:

```go
package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFakeClock_AdvanceMovesTimeForward(t *testing.T) {
	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	c := NewFakeClock(start)

	require.Equal(t, start, c.Now())

	c.Advance(30 * time.Second)
	require.Equal(t, start.Add(30*time.Second), c.Now())

	c.Advance(2 * time.Minute)
	require.Equal(t, start.Add(30*time.Second+2*time.Minute), c.Now())
}

func TestRealClock_ReturnsCurrentTime(t *testing.T) {
	c := RealClock{}
	before := time.Now()
	got := c.Now()
	after := time.Now()

	require.False(t, got.Before(before))
	require.False(t, got.After(after))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
just backend test ./internal/ratelimit/...
```

Expected: FAIL — `package ratelimit` does not exist yet.

- [ ] **Step 3: Implement clock.go**

Create `backend/internal/ratelimit/clock.go`:

```go
package ratelimit

import (
	"sync"
	"time"
)

// Clock is a minimal time source. The rate-limiter accepts an injected clock
// so tests can advance time deterministically.
type Clock interface {
	Now() time.Time
}

// RealClock wraps time.Now.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// FakeClock is a test clock advanced manually via Advance.
type FakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{t: start}
}

func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
just backend test ./internal/ratelimit/...
```

Expected: PASS, both tests.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ratelimit/clock.go backend/internal/ratelimit/clock_test.go
git commit -m "feat(tra-395): add clock abstraction for rate limiter"
```

---

## Task 3: Limiter — basic Allow on a fresh key

**Files:**
- Create: `backend/internal/ratelimit/limiter.go`
- Modify/extend: `backend/internal/ratelimit/limiter_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/ratelimit/limiter_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
just backend test ./internal/ratelimit/...
```

Expected: FAIL — `NewLimiter`, `Config`, `Limiter.Allow`, `Limiter.Close`, `Decision` all undefined.

- [ ] **Step 3: Implement limiter.go**

Create `backend/internal/ratelimit/limiter.go`:

```go
package ratelimit

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// Decision is returned from Allow for every request.
type Decision struct {
	Allowed    bool
	Limit      int           // steady-state requests per minute
	Remaining  int           // whole tokens left after this call
	ResetAt    time.Time     // when the bucket will be fully refilled
	RetryAfter time.Duration // 0 when allowed; else wait time until next token
}

// Config configures a Limiter.
type Config struct {
	RatePerMinute int
	Burst         int
	IdleTTL       time.Duration
	SweepInterval time.Duration
	Clock         Clock
}

// DefaultConfig returns the production defaults: 60/min, 120 burst, 1h idle
// TTL, 10m sweep interval, RealClock.
func DefaultConfig() Config {
	return Config{
		RatePerMinute: 60,
		Burst:         120,
		IdleTTL:       time.Hour,
		SweepInterval: 10 * time.Minute,
		Clock:         RealClock{},
	}
}

type bucket struct {
	lim      *rate.Limiter
	lastSeen atomic.Int64 // unix nanos
}

// Limiter is a per-key token-bucket rate limiter. Zero value is not usable;
// construct with NewLimiter.
type Limiter struct {
	cfg     Config
	buckets sync.Map // key string -> *bucket
	stop    chan struct{}
	done    chan struct{}
}

// NewLimiter constructs a Limiter and starts its background sweeper goroutine.
// Caller must invoke Close to stop the sweeper.
func NewLimiter(cfg Config) *Limiter {
	if cfg.Clock == nil {
		cfg.Clock = RealClock{}
	}
	l := &Limiter{
		cfg:  cfg,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go l.sweepLoop()
	return l
}

// Close stops the sweeper goroutine. Safe to call once; subsequent calls panic.
func (l *Limiter) Close() {
	close(l.stop)
	<-l.done
}

// Allow consumes one token for the given key and returns the decision.
func (l *Limiter) Allow(key string) Decision {
	now := l.cfg.Clock.Now()
	b := l.bucketFor(key)
	b.lastSeen.Store(now.UnixNano())

	allowed := b.lim.AllowN(now, 1)
	tokens := b.lim.TokensAt(now)
	if tokens < 0 {
		tokens = 0
	}

	perSec := float64(l.cfg.RatePerMinute) / 60.0

	d := Decision{
		Allowed:   allowed,
		Limit:     l.cfg.RatePerMinute,
		Remaining: int(math.Floor(tokens)),
	}

	deficit := float64(l.cfg.Burst) - tokens
	if deficit < 0 {
		deficit = 0
	}
	d.ResetAt = now.Add(time.Duration(deficit / perSec * float64(time.Second)))

	if !allowed {
		needed := 1.0 - tokens
		if needed < 0 {
			needed = 0
		}
		retry := time.Duration(needed / perSec * float64(time.Second))
		if retry <= 0 {
			retry = time.Second
		}
		d.RetryAfter = retry
	}
	return d
}

func (l *Limiter) bucketFor(key string) *bucket {
	if v, ok := l.buckets.Load(key); ok {
		return v.(*bucket)
	}
	fresh := &bucket{
		lim: rate.NewLimiter(
			rate.Limit(float64(l.cfg.RatePerMinute)/60.0),
			l.cfg.Burst,
		),
	}
	actual, _ := l.buckets.LoadOrStore(key, fresh)
	return actual.(*bucket)
}

func (l *Limiter) sweepLoop() {
	defer close(l.done)
	if l.cfg.SweepInterval <= 0 {
		<-l.stop
		return
	}
	t := time.NewTicker(l.cfg.SweepInterval)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
			l.sweep()
		}
	}
}

// sweep drops buckets idle for longer than IdleTTL. Exported package-internally
// for tests; production callers use the background sweeper.
func (l *Limiter) sweep() {
	cutoff := l.cfg.Clock.Now().Add(-l.cfg.IdleTTL).UnixNano()
	l.buckets.Range(func(k, v any) bool {
		b := v.(*bucket)
		if b.lastSeen.Load() < cutoff {
			l.buckets.Delete(k)
		}
		return true
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
just backend test ./internal/ratelimit/...
```

Expected: PASS on both tests plus earlier clock tests.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ratelimit/limiter.go backend/internal/ratelimit/limiter_test.go
git commit -m "feat(tra-395): token-bucket limiter with fresh-key Allow"
```

---

## Task 4: Limiter — burst exhaustion and RetryAfter

**Files:**
- Modify: `backend/internal/ratelimit/limiter_test.go`

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/ratelimit/limiter_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails or passes**

```bash
just backend test ./internal/ratelimit/... -run TestLimiter_ExhaustedBurstIsDenied
```

Expected: PASS — the implementation in Task 3 already covers this. If it FAILS, fix the `RetryAfter` calculation in `limiter.go` before proceeding.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/ratelimit/limiter_test.go
git commit -m "test(tra-395): burst exhaustion returns RetryAfter"
```

---

## Task 5: Limiter — refill after clock advance

**Files:**
- Modify: `backend/internal/ratelimit/limiter_test.go`

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/ratelimit/limiter_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it passes**

```bash
just backend test ./internal/ratelimit/... -run TestLimiter_RefillsOverTime
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/ratelimit/limiter_test.go
git commit -m "test(tra-395): bucket refills as clock advances"
```

---

## Task 6: Limiter — ResetAt and key independence

**Files:**
- Modify: `backend/internal/ratelimit/limiter_test.go`

- [ ] **Step 1: Write the failing test**

Append:

```go
func TestLimiter_ResetAtReflectsTimeToFullRefill(t *testing.T) {
	start := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)
	lim := newTestLimiter(clock)
	defer lim.Close()

	// Drain the full burst (120 tokens).
	for i := 0; i < 120; i++ {
		lim.Allow("key-a")
	}

	d := lim.Allow("key-a") // denied; bucket at 0
	// At 60/min = 1/sec, refilling 120 tokens takes 120s.
	expected := start.Add(120 * time.Second)
	require.WithinDuration(t, expected, d.ResetAt, 100*time.Millisecond)
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

	// key-b untouched — should still have full burst.
	d := lim.Allow("key-b")
	require.True(t, d.Allowed)
	require.Equal(t, 119, d.Remaining)
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
just backend test ./internal/ratelimit/... -run 'TestLimiter_ResetAt|TestLimiter_KeysAreIndependent'
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/ratelimit/limiter_test.go
git commit -m "test(tra-395): ResetAt accuracy and key independence"
```

---

## Task 7: Limiter — sweeper evicts idle buckets

**Files:**
- Modify: `backend/internal/ratelimit/limiter_test.go`

- [ ] **Step 1: Write the failing test**

Append:

```go
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
	require.Equal(t, 119, d.Remaining, "fresh bucket after eviction")
}
```

- [ ] **Step 2: Run test to verify it passes**

```bash
just backend test ./internal/ratelimit/... -run TestLimiter_Swept
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/ratelimit/limiter_test.go
git commit -m "test(tra-395): sweeper evicts idle buckets"
```

---

## Task 8: Middleware — session-auth bypass

**Files:**
- Create: `backend/internal/middleware/ratelimit.go`
- Create: `backend/internal/middleware/ratelimit_test.go`

- [ ] **Step 1: Write the failing test**

Create `backend/internal/middleware/ratelimit_test.go`:

```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/ratelimit"
)

func newTestRateLimiter(t *testing.T) (*ratelimit.Limiter, *ratelimit.FakeClock) {
	t.Helper()
	clock := ratelimit.NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := ratelimit.NewLimiter(ratelimit.Config{
		RatePerMinute: 60,
		Burst:         120,
		IdleTTL:       time.Hour,
		SweepInterval: 24 * time.Hour,
		Clock:         clock,
	})
	t.Cleanup(func() { lim.Close() })
	return lim, clock
}

func TestRateLimit_SessionAuthBypassesRateLimiting(t *testing.T) {
	lim, _ := newTestRateLimiter(t)

	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// No APIKeyPrincipal on context — simulates session-authenticated request.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	rec := httptest.NewRecorder()

	RateLimit(lim)(next).ServeHTTP(rec, req)

	require.True(t, handlerCalled, "session auth request must pass through")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Get("X-RateLimit-Limit"), "no rate-limit headers for session auth")
	require.Empty(t, rec.Header().Get("X-RateLimit-Remaining"))
	require.Empty(t, rec.Header().Get("X-RateLimit-Reset"))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
just backend test ./internal/middleware/... -run TestRateLimit_SessionAuthBypassesRateLimiting
```

Expected: FAIL — `RateLimit` is undefined.

- [ ] **Step 3: Implement the middleware skeleton**

Create `backend/internal/middleware/ratelimit.go`:

```go
package middleware

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/ratelimit"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// RateLimit returns a middleware that enforces per-key rate limits on
// API-key-authenticated requests. Session-authenticated requests (identified
// by the absence of an APIKeyPrincipal on the context) pass through untouched.
//
// Emits X-RateLimit-Limit, X-RateLimit-Remaining, and X-RateLimit-Reset on
// every rate-limited response. On denial, emits 429 with Retry-After and a
// standard error envelope (type=rate_limited).
func RateLimit(lim *ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := GetAPIKeyPrincipal(r)
			if p == nil {
				next.ServeHTTP(w, r)
				return
			}

			d := lim.Allow(p.JTI)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(d.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(d.ResetAt.Unix(), 10))

			if !d.Allowed {
				retrySec := int(math.Ceil(d.RetryAfter.Seconds()))
				if retrySec < 1 {
					retrySec = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retrySec))

				reqID := GetRequestID(r.Context())
				logger.Get().Warn().
					Str("request_id", reqID).
					Str("jti", p.JTI).
					Int("org_id", p.OrgID).
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Msg("rate limit exceeded")

				httputil.WriteJSONError(w, r, http.StatusTooManyRequests,
					errors.ErrRateLimited,
					"Rate limit exceeded",
					fmt.Sprintf("Retry after %d seconds", retrySec),
					reqID)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
just backend test ./internal/middleware/... -run TestRateLimit_SessionAuthBypassesRateLimiting
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/middleware/ratelimit.go backend/internal/middleware/ratelimit_test.go
git commit -m "feat(tra-395): rate-limit middleware skeleton, session bypass"
```

---

## Task 9: Middleware — allowed path sets headers, 429 path returns error envelope

**Files:**
- Modify: `backend/internal/middleware/ratelimit_test.go`

- [ ] **Step 1: Write the failing tests**

Two edits to `backend/internal/middleware/ratelimit_test.go`:

1. Update the top-of-file import block to include `"context"` and `"encoding/json"`. After editing, it should read:

```go
import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/ratelimit"
)
```

2. Append these tests and helper to the bottom of the file:

```go
func requestWithAPIKey(jti string, orgID int) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	p := &APIKeyPrincipal{OrgID: orgID, JTI: jti, Scopes: []string{"assets:read"}}
	ctx := context.WithValue(req.Context(), APIKeyPrincipalKey, p)
	return req.WithContext(ctx)
}

func TestRateLimit_AllowedRequestSetsHeaders(t *testing.T) {
	lim, _ := newTestRateLimiter(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	RateLimit(lim)(next).ServeHTTP(rec, requestWithAPIKey("jti-alpha", 42))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "60", rec.Header().Get("X-RateLimit-Limit"))
	require.Equal(t, "119", rec.Header().Get("X-RateLimit-Remaining"))
	require.NotEmpty(t, rec.Header().Get("X-RateLimit-Reset"))
	require.Empty(t, rec.Header().Get("Retry-After"), "allowed responses have no Retry-After")
}

func TestRateLimit_DeniedRequestReturns429WithEnvelope(t *testing.T) {
	lim, _ := newTestRateLimiter(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run when rate limit is exceeded")
	})

	// Drain the burst for this principal.
	drain := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	for i := 0; i < 120; i++ {
		rec := httptest.NewRecorder()
		RateLimit(lim)(drain).ServeHTTP(rec, requestWithAPIKey("jti-alpha", 42))
		require.Equal(t, http.StatusOK, rec.Code, "request %d should succeed", i+1)
	}

	// 121st request — denied.
	rec := httptest.NewRecorder()
	RateLimit(lim)(next).ServeHTTP(rec, requestWithAPIKey("jti-alpha", 42))

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "1", rec.Header().Get("Retry-After"))
	require.Equal(t, "60", rec.Header().Get("X-RateLimit-Limit"))
	require.Equal(t, "0", rec.Header().Get("X-RateLimit-Remaining"))

	var body struct {
		Error struct {
			Type     string `json:"type"`
			Title    string `json:"title"`
			Status   int    `json:"status"`
			Detail   string `json:"detail"`
			Instance string `json:"instance"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "rate_limited", body.Error.Type)
	require.Equal(t, "Rate limit exceeded", body.Error.Title)
	require.Equal(t, 429, body.Error.Status)
	require.Equal(t, "Retry after 1 seconds", body.Error.Detail)
	require.Equal(t, "/api/v1/assets", body.Error.Instance)
}

func TestRateLimit_TwoPrincipalsIndependent(t *testing.T) {
	lim, _ := newTestRateLimiter(t)

	drain := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Drain key-a.
	for i := 0; i < 120; i++ {
		rec := httptest.NewRecorder()
		RateLimit(lim)(drain).ServeHTTP(rec, requestWithAPIKey("key-a", 1))
	}

	// key-a denied.
	recA := httptest.NewRecorder()
	RateLimit(lim)(drain).ServeHTTP(recA, requestWithAPIKey("key-a", 1))
	require.Equal(t, http.StatusTooManyRequests, recA.Code)

	// key-b still healthy.
	recB := httptest.NewRecorder()
	RateLimit(lim)(drain).ServeHTTP(recB, requestWithAPIKey("key-b", 2))
	require.Equal(t, http.StatusOK, recB.Code)
	require.Equal(t, "119", recB.Header().Get("X-RateLimit-Remaining"))
}
```

- [ ] **Step 2: Run tests to verify they pass**

```bash
just backend test ./internal/middleware/... -run TestRateLimit_
```

Expected: all four `TestRateLimit_*` tests PASS.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/middleware/ratelimit_test.go
git commit -m "test(tra-395): middleware allowed/denied paths + per-key isolation"
```

---

## Task 10: Wire middleware into router + integration smoke test

**Files:**
- Modify: `backend/internal/cmd/serve/router.go`
- Create: `backend/internal/cmd/serve/ratelimit_smoke_test.go`

- [ ] **Step 1: Read the current router file**

```bash
```

(This is a reminder step; the plan already quoted the relevant section from `router.go:91-110`.)

- [ ] **Step 2: Modify router.go**

Open `backend/internal/cmd/serve/router.go`. Near the top of `NewRouter` (or wherever the handlers are constructed — around line 35+), add the limiter construction. Then add `middleware.RateLimit(rl)` to the EitherAuth group (before `SentryContext`).

**Add import** at the top of `router.go`:

```go
"github.com/trakrf/platform/backend/internal/ratelimit"
```

**Add limiter construction** near other top-of-function setup (before routes are registered). Pick a location just before the `r.With(middleware.APIKeyAuth(store)).Get("/api/v1/orgs/me", ...)` line so the lifecycle is obvious:

```go
// Per-key rate limiter for API-key-authenticated requests (TRA-395).
// /orgs/me is intentionally excluded as a health-check exemption.
// Limiter lives for the process lifetime; its sweeper runs in a goroutine.
rl := ratelimit.NewLimiter(ratelimit.DefaultConfig())
```

**Modify the EitherAuth group** (currently `backend/internal/cmd/serve/router.go:95-110`) to add `r.Use(middleware.RateLimit(rl))`:

Replace:

```go
r.Group(func(r chi.Router) {
    r.Use(middleware.EitherAuth(store))
    r.Use(middleware.SentryContext)

    r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets", assetsHandler.ListAssets)
```

With:

```go
r.Group(func(r chi.Router) {
    r.Use(middleware.EitherAuth(store))
    r.Use(middleware.RateLimit(rl))
    r.Use(middleware.SentryContext)

    r.With(middleware.RequireScope("assets:read")).Get("/api/v1/assets", assetsHandler.ListAssets)
```

- [ ] **Step 3: Write the smoke test**

Create `backend/internal/cmd/serve/ratelimit_smoke_test.go`:

```go
package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/ratelimit"
)

// TestRateLimit_MountedOnEitherAuthGroup verifies that a handler mounted under
// the same group as the public API read endpoints receives rate-limit headers
// when invoked with an APIKeyPrincipal on the context.
//
// This is an isolated smoke test — it does not boot the full router (which
// requires DB + storage) but exercises the same middleware chain shape.
func TestRateLimit_MountedOnEitherAuthGroup(t *testing.T) {
	clock := ratelimit.NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := ratelimit.NewLimiter(ratelimit.Config{
		RatePerMinute: 60,
		Burst:         120,
		IdleTTL:       time.Hour,
		SweepInterval: 24 * time.Hour,
		Clock:         clock,
	})
	defer lim.Close()

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(middleware.RateLimit(lim))
		r.Get("/ping", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	p := &middleware.APIKeyPrincipal{OrgID: 7, JTI: "smoke-jti", Scopes: []string{"assets:read"}}
	req = req.WithContext(contextWithPrincipal(req.Context(), p))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "60", rec.Header().Get("X-RateLimit-Limit"))
	require.Equal(t, "119", rec.Header().Get("X-RateLimit-Remaining"))
}
```

- [ ] **Step 4: Add the `contextWithPrincipal` helper and its `context` import**

Two edits to the same test file:

1. Add `"context"` to the top-of-file import block. After editing, the import block should look like:

```go
import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/ratelimit"
)
```

2. Add this helper function at the bottom of the file (below `TestRateLimit_MountedOnEitherAuthGroup`):

```go
// contextWithPrincipal mirrors what APIKeyAuth does internally. Kept inline
// rather than exported from middleware to avoid widening the package surface.
func contextWithPrincipal(ctx context.Context, p *middleware.APIKeyPrincipal) context.Context {
	return context.WithValue(ctx, middleware.APIKeyPrincipalKey, p)
}
```

Note: this assumes `APIKeyPrincipalKey` is exported — it is, see `backend/internal/middleware/apikey.go:23`.

- [ ] **Step 5: Run the smoke test**

```bash
just backend test ./internal/cmd/serve/... -run TestRateLimit_MountedOnEitherAuthGroup
```

Expected: PASS.

- [ ] **Step 6: Run the full middleware + ratelimit + serve test suites to confirm no regressions**

```bash
just backend test ./internal/middleware/... ./internal/ratelimit/... ./internal/cmd/serve/...
```

Expected: all green.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/cmd/serve/router.go backend/internal/cmd/serve/ratelimit_smoke_test.go
git commit -m "feat(tra-395): wire rate-limit middleware into public API group"
```

---

## Task 11: OpenAPI — add `X-RateLimit-*` header annotations (via swag)

**Background:** `docs/api/openapi.public.yaml` is **generated** from swag annotations in handler files by `just backend api-spec`. Editing the YAML directly will be overwritten. All edits in this task go in the `// @Success` / `// @Failure` comment blocks above handler functions.

The `@Failure 429 {object} modelerrors.ErrorResponse "rate_limited"` line already exists on public handlers (verified). This task only adds `@Header` annotations for the three `X-RateLimit-*` headers on success responses.

**Files:**
- Modify: handler files containing the 6 public read endpoints. The current list (from `backend/internal/cmd/serve/router.go:99-110`):
  - `backend/internal/handlers/assets/assets.go` — `ListAssets`, `GetAssetByIdentifier`
  - `backend/internal/handlers/locations/locations.go` — `ListLocations`, `GetLocationByIdentifier`
  - `backend/internal/handlers/reports/reports.go` (or wherever `ListCurrentLocations` / `GetAssetHistory` live — use `grep` to confirm)

- [ ] **Step 1: Confirm handler locations**

```bash
grep -rn "func.*ListCurrentLocations\|func.*GetAssetHistory\b" backend/internal/handlers
```

Record the exact files. Apply the annotation edits there.

- [ ] **Step 2: Add `@Header` annotations on each public read handler**

For each of the 6 handlers, locate the comment block that ends with the handler's Go signature. Between the existing `@Success` line and the `@Failure` lines, add three header lines:

```go
// @Header       200  {integer}  X-RateLimit-Limit      "Steady-state requests/min for this API key"
// @Header       200  {integer}  X-RateLimit-Remaining  "Tokens left in bucket at response time"
// @Header       200  {integer}  X-RateLimit-Reset      "Unix timestamp when bucket fully refills"
```

If a handler uses a `@Success` code other than 200 (e.g., 201 for POSTs — not applicable in this read-only ticket), match the code.

Example — current comment block in `assets.go` for `ListAssets` (approximate; confirm exact location):

```go
// ListAssets lists assets for the authenticated org.
// @Summary      List assets
// @Tags         assets
// @Produce      json
// @Success      200  {object}  ListAssetsResponse
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[assets:read]
// @Router       /api/v1/assets [get]
```

After edit:

```go
// ListAssets lists assets for the authenticated org.
// @Summary      List assets
// @Tags         assets
// @Produce      json
// @Success      200  {object}  ListAssetsResponse
// @Header       200  {integer}  X-RateLimit-Limit      "Steady-state requests/min for this API key"
// @Header       200  {integer}  X-RateLimit-Remaining  "Tokens left in bucket at response time"
// @Header       200  {integer}  X-RateLimit-Reset      "Unix timestamp when bucket fully refills"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[assets:read]
// @Router       /api/v1/assets [get]
```

Apply to all six public read handlers. Do **not** touch `GetOrgMe` — `/api/v1/orgs/me` is rate-limit-exempt by design.

- [ ] **Step 3: Regenerate the OpenAPI specs**

```bash
just backend api-spec
```

Expected: the command completes cleanly. `docs/api/openapi.public.yaml` and `.json` are regenerated with new header entries on the 6 paths.

- [ ] **Step 4: Verify header presence in the generated spec**

```bash
grep -A 4 "X-RateLimit-Limit" docs/api/openapi.public.yaml | head -40
```

Expected: 6 matches (one per path), each with the description text.

- [ ] **Step 5: Lint the generated spec**

```bash
just backend api-lint
```

Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handlers docs/api/openapi.public.json docs/api/openapi.public.yaml
git commit -m "docs(tra-395): annotate X-RateLimit-* response headers in OpenAPI"
```

---

## Task 12: Final verification

- [ ] **Step 1: Run everything**

```bash
just backend test
just backend lint
```

Expected: all green. No `go vet` or `golangci-lint` complaints.

- [ ] **Step 2: Manual sanity check**

Start the backend locally:

```bash
just backend run
```

In another terminal, hit an API-key endpoint with a valid bearer token (re-use one from the API-key dev-fixture if available, or create one via the UI) and verify headers are present:

```bash
curl -i -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/assets | grep -i ratelimit
```

Expected:

```
X-RateLimit-Limit: 60
X-RateLimit-Remaining: 119
X-RateLimit-Reset: <unix-ts>
```

Verify `/api/v1/orgs/me` does **not** emit those headers:

```bash
curl -i -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/orgs/me | grep -i ratelimit
```

Expected: no output.

- [ ] **Step 3: Push and open PR**

Per project convention (CLAUDE.md), push the branch and open a PR to `main` — do not merge locally.

```bash
git push -u origin feature/tra-395-rate-limiting
gh pr create --title "feat(tra-395): rate limiting middleware with per-key token buckets" --body "$(cat <<'EOF'
## Summary
- Adds `backend/internal/ratelimit` — pure-Go token-bucket limiter with clock injection and TTL-based idle-bucket eviction.
- Adds `middleware.RateLimit(lim)` — HTTP adapter keyed on `APIKeyPrincipal.JTI`. Session-authenticated requests bypass; no API-key principal means no rate-limit application.
- Emits `X-RateLimit-{Limit,Remaining,Reset}` on every API-key-authenticated response and `Retry-After` on 429.
- 429 body matches published docs at `docs.preview.trakrf.id/docs/api/rate-limits/` exactly.
- Defaults: 60 req/min steady, 120 burst, 1h idle eviction. Tier differentiation deferred to TRA-337 (comment posted on that ticket).
- Mounted on the EitherAuth group. `GET /api/v1/orgs/me` stays outside the rate-limited group as a health-check exemption.
- OpenAPI updated with 429 + `X-RateLimit-*` header schemas.

## Test plan
- [ ] `just backend test ./internal/ratelimit/... ./internal/middleware/... ./internal/cmd/serve/...` — all green
- [ ] Manual curl against a preview deploy: rate-limit headers present on `/assets`, absent on `/orgs/me`
- [ ] Generate 125 requests in quick succession, verify a 429 with `Retry-After: 1` once burst is exhausted

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR opened with preview deploy link.

---

## Self-Review Checklist

**Spec coverage:**

| Spec section | Task |
|---|---|
| `ratelimit` package module layout | Tasks 2, 3 |
| `Clock` interface + `FakeClock` | Task 2 |
| `Decision` struct | Task 3 |
| `Config` + `DefaultConfig` | Task 3 |
| `Limiter.Allow` logic (bucket load, lastSeen, AllowN, Remaining, ResetAt, RetryAfter) | Tasks 3, 4, 5, 6 |
| Sweeper (start, stop, evict idle, fresh bucket on return) | Task 7 |
| Thread-safety via `sync.Map` + `atomic.Int64` | Task 3 (implicit in code) |
| HTTP middleware: session bypass | Task 8 |
| HTTP middleware: `X-RateLimit-*` headers on success | Task 9 |
| HTTP middleware: 429 body, `Retry-After`, warning log | Task 9 |
| Mount points in `router.go`, `/orgs/me` exclusion | Task 10 |
| Integration smoke test | Task 10 |
| OpenAPI updates | Task 11 |
| Final verification + PR | Task 12 |

**Placeholder scan:** No "TBD"/"TODO"/"fill in" — every step shows concrete code or exact commands.

**Type consistency:** `Decision`, `Config`, `Limiter`, `bucket` names are stable across tasks. `ratelimit.NewLimiter`, `ratelimit.NewFakeClock`, `ratelimit.Config`, `ratelimit.FakeClock` are the exported identifiers used everywhere.

**Gaps considered and explicitly deferred:**
- Prometheus metrics — out of scope, noted in spec.
- Tier-based limits — TRA-337 scope, comment posted.
- Redis / multi-instance — single-instance Railway is fine for launch.
- Env-var overrides — YAGNI.
