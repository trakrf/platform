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
