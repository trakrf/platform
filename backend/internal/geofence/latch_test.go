package geofence

import (
	"sync"
	"testing"
	"time"
)

const testTTL = time.Minute

func TestLatch_FirstSightFires(t *testing.T) {
	l := newLatch(0, nil)
	defer l.Close()

	if !l.admit(latchKey(1, 2, "EPC"), time.Unix(100, 0), testTTL) {
		t.Fatal("first sight of a key must fire")
	}
}

func TestLatch_RepeatWithinTTLSuppressed(t *testing.T) {
	l := newLatch(0, nil)
	defer l.Close()

	key := latchKey(1, 2, "EPC")
	base := time.Unix(100, 0)
	if !l.admit(key, base, testTTL) {
		t.Fatal("first sight must fire")
	}
	if l.admit(key, base.Add(10*time.Second), testTTL) {
		t.Fatal("repeat within TTL must be suppressed")
	}
	if l.admit(key, base.Add(59*time.Second), testTTL) {
		t.Fatal("repeat still within TTL must be suppressed")
	}
}

func TestLatch_ReArmsAfterTTL(t *testing.T) {
	l := newLatch(0, nil)
	defer l.Close()

	key := latchKey(1, 2, "EPC")
	base := time.Unix(100, 0)
	if !l.admit(key, base, testTTL) {
		t.Fatal("first sight must fire")
	}
	// Silent for longer than the TTL, then re-enter.
	if !l.admit(key, base.Add(90*time.Second), testTTL) {
		t.Fatal("re-entry after absence > TTL must fire again")
	}
	// And immediately latched again.
	if l.admit(key, base.Add(91*time.Second), testTTL) {
		t.Fatal("immediately after a re-fire must be suppressed")
	}
}

func TestLatch_PerCallTTL(t *testing.T) {
	l := newLatch(0, nil)
	defer l.Close()
	key := latchKey(1, 2, "EPC")
	base := time.Unix(1000, 0)

	if !l.admit(key, base, 10*time.Second) {
		t.Fatal("first sight must fire")
	}
	// 30s later re-arms under a 10s window (the per-output age-out).
	if !l.admit(key, base.Add(30*time.Second), 10*time.Second) {
		t.Fatal("30s gap should re-arm a 10s ttl")
	}
}

func TestLatch_DistinctKeysIndependent(t *testing.T) {
	l := newLatch(0, nil)
	defer l.Close()

	now := time.Unix(100, 0)
	if !l.admit(latchKey(1, 2, "A"), now, testTTL) {
		t.Fatal("key A first sight must fire")
	}
	if !l.admit(latchKey(1, 2, "B"), now, testTTL) {
		t.Fatal("key B first sight must fire (independent of A)")
	}
	if !l.admit(latchKey(1, 3, "A"), now, testTTL) {
		t.Fatal("same epc at a different output must fire (independent key)")
	}
}

func TestLatch_SweepEvictsIdle(t *testing.T) {
	clk := NewFakeClock(time.Unix(100, 0))
	l := newLatch(0, clk) // sweepInterval 0 -> drive sweep() manually
	defer l.Close()

	l.admit(latchKey(1, 2, "A"), clk.Now(), testTTL)
	if l.size() != 1 {
		t.Fatalf("expected 1 live entry, got %d", l.size())
	}

	// Not yet aged out.
	clk.Advance(30 * time.Second)
	l.sweep()
	if l.size() != 1 {
		t.Fatalf("entry within TTL must survive sweep, got size %d", l.size())
	}

	// Aged out.
	clk.Advance(90 * time.Second)
	l.sweep()
	if l.size() != 0 {
		t.Fatalf("idle entry must be evicted, got size %d", l.size())
	}
}

func TestLatch_ConcurrentFirstSightFiresOnce(t *testing.T) {
	l := newLatch(0, nil)
	defer l.Close()

	const goroutines = 64
	key := latchKey(1, 2, "RACE")
	now := time.Unix(100, 0)

	var fires int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if l.admit(key, now, testTTL) {
				mu.Lock()
				fires++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if fires != 1 {
		t.Fatalf("concurrent first-sight admits must fire exactly once, got %d", fires)
	}
}
