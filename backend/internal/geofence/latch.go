package geofence

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// latch is the per-(org, boundary, epc) dedup cache. It mirrors the aging
// pattern in ratelimit/limiter.go (sync.Map + atomic lastSeen + a background
// sweeper) but takes the observation time from the caller (server receive time)
// rather than a clock, so admit decisions are deterministic and clock-free.
//
// admit returns true ("fire") on first sight of a key and again once the key has
// been absent longer than ttl (re-entry); it returns false ("suppress, latched")
// while the tag is present. The sweeper only frees memory for keys that have
// aged out; expiry is also enforced on access, so the sweeper never affects
// correctness — its clock is wall-clock and independent of the admit timestamps.
type latch struct {
	clk      Clock
	sweepInt time.Duration

	seen sync.Map // key string -> *latchEntry

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

type latchEntry struct {
	lastSeen atomic.Int64 // unix nanos of the most recent admit() call
	ttlNanos atomic.Int64 // re-arm window for this key (per-output age-out)
}

// newLatch constructs a latch and starts its sweeper goroutine. A non-positive
// sweepInterval disables periodic GC (the loop just waits for Close); admit-time
// expiry still works. Caller must Close to stop the sweeper.
func newLatch(sweepInterval time.Duration, clk Clock) *latch {
	if clk == nil {
		clk = RealClock{}
	}
	l := &latch{
		clk:      clk,
		sweepInt: sweepInterval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go l.sweepLoop()
	return l
}

// latchKey builds the egress dedup key, scoped per (org, output device, epc).
// EPC is opaque text; org and output id are ints.
func latchKey(orgID, outputID int, epc string) string {
	return fmt.Sprintf("%d:%d:%s", orgID, outputID, epc)
}

// admit records an observation of key at time now and reports whether it should
// fire, using the caller-supplied re-arm window (the output's age-out). It is
// safe for concurrent use: LoadOrStore makes exactly one goroutine win
// first-sight, and the atomic Swap makes exactly one goroutine observe an
// aged-out predecessor on a concurrent re-entry.
func (l *latch) admit(key string, now time.Time, ttl time.Duration) bool {
	nowNanos := now.UnixNano()

	fresh := &latchEntry{}
	fresh.lastSeen.Store(nowNanos)
	fresh.ttlNanos.Store(ttl.Nanoseconds())
	v, loaded := l.seen.LoadOrStore(key, fresh)
	if !loaded {
		return true // first sight
	}

	e := v.(*latchEntry)
	e.ttlNanos.Store(ttl.Nanoseconds())
	prev := e.lastSeen.Swap(nowNanos)
	return nowNanos-prev > ttl.Nanoseconds() // re-armed after absence
}

func (l *latch) sweepLoop() {
	defer close(l.done)
	if l.sweepInt <= 0 {
		<-l.stop
		return
	}
	t := time.NewTicker(l.sweepInt)
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

// sweep drops entries not seen within their own ttl. Exported package-internally
// for tests.
func (l *latch) sweep() {
	now := l.clk.Now().UnixNano()
	l.seen.Range(func(k, v any) bool {
		e := v.(*latchEntry)
		if e.lastSeen.Load() < now-e.ttlNanos.Load() {
			l.seen.Delete(k)
		}
		return true
	})
}

// Close stops the sweeper. Safe to call multiple times.
func (l *latch) Close() {
	l.closeOnce.Do(func() {
		close(l.stop)
		<-l.done
	})
}

// size reports the number of live latch entries (test helper).
func (l *latch) size() int {
	n := 0
	l.seen.Range(func(_, _ any) bool { n++; return true })
	return n
}
