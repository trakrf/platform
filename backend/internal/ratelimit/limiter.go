package ratelimit

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// Decision is returned from Allow for every request.
//
// Header semantics (TRA-878, "Option A" — POLS for professional integrators):
// Limit is the TRUE ceiling — the maximum number of requests a caller can make
// in an instantaneous burst before a 429 — which equals the token bucket's
// burst capacity. Remaining is the live token count and decrements 1:1 on every
// request, from Limit down to 0, with no flat "burst margin" masking. A generic
// SDK's "back off as Remaining approaches 0" logic therefore works as written.
//
// The SUSTAINED rate (which is lower than the burst ceiling) is communicated
// separately via PolicyQuota/PolicyWindowSec, surfaced as the RateLimit-Policy
// header. Sophisticated clients read the policy; naive clients pace off the
// Remaining countdown and the bucket self-regulates them toward the sustained
// rate via refill. Both are honest.
type Decision struct {
	Allowed         bool
	Limit           int           // ceiling: max requests in a burst before 429 (== bucket burst)
	Remaining       int           // live tokens left before throttling; 0 ≤ Remaining ≤ Limit
	ResetAt         time.Time     // when Remaining will next equal Limit (== now if already there)
	RetryAfter      time.Duration // 0 when allowed; else wait time until next token
	PolicyQuota     int           // sustained requests allowed per PolicyWindowSec
	PolicyWindowSec int           // sustained-rate window, in seconds
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
	cfg       Config
	buckets   sync.Map // key string -> *bucket
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
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

// Close stops the sweeper goroutine. Safe to call multiple times; subsequent
// calls are no-ops.
func (l *Limiter) Close() {
	l.closeOnce.Do(func() {
		close(l.stop)
		<-l.done
	})
}

// AnonDecision returns the decision an unrecognized caller would receive: an
// untouched full bucket, no consumption charged. Used by DefaultRateLimitHeaders
// to advertise rate-limit policy on responses where the principal can't be
// identified (auth-failure 401s, 404s on unknown /api/v1/* paths). Remaining
// equals the ceiling because nothing has been consumed.
func (l *Limiter) AnonDecision() Decision {
	return Decision{
		Allowed:         true,
		Limit:           l.cfg.Burst,
		Remaining:       l.cfg.Burst,
		ResetAt:         l.cfg.Clock.Now(),
		PolicyQuota:     l.cfg.RatePerMinute,
		PolicyWindowSec: 60,
	}
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
	// Limit is the true ceiling — the bucket's burst capacity, i.e. the most
	// requests a caller can make before a 429. Remaining is the live token count
	// and decrements 1:1 from the first request (no burst-margin masking), so a
	// generic client's "back off as Remaining nears 0" works as written. The
	// lower sustained rate is advertised separately via the RateLimit-Policy
	// fields below. (TRA-878 Option A.)
	limit := l.cfg.Burst

	remaining := int(math.Floor(tokens))
	if remaining > limit {
		remaining = limit
	}
	if remaining < 0 {
		remaining = 0
	}

	// ResetAt = wall-clock time when Remaining will next equal Limit (the
	// ceiling). When the bucket is full the caller already has full quota, so
	// ResetAt is now (sleep(reset-now) → 0, the correct no-op). Otherwise it's
	// the time by which the bucket will have refilled back to the ceiling.
	var resetAt time.Time
	if tokens >= float64(limit) {
		resetAt = now
	} else {
		deficit := float64(limit) - tokens
		resetAt = now.Add(time.Duration(deficit / perSec * float64(time.Second)))
	}

	d := Decision{
		Allowed:         allowed,
		Limit:           limit,
		Remaining:       remaining,
		ResetAt:         resetAt,
		PolicyQuota:     l.cfg.RatePerMinute,
		PolicyWindowSec: 60,
	}

	if !allowed {
		needed := 1.0 - tokens
		if needed < 0 {
			needed = 0
		}
		retry := time.Duration(needed / perSec * float64(time.Second))
		// Load-bearing: under concurrent same-key bursts, `needed` can round to
		// zero and produce a 0ns duration. A zero Retry-After would be a useless
		// hint to clients (and is out of spec for RFC 9110). Floor at 1s.
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
