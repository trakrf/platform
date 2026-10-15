package geofence

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPulseGuard_DropsInsideWindowAllowsAtEnd(t *testing.T) {
	g := newPulseGuard()
	at := time.Unix(1000, 0)
	d := 5 * time.Second

	if !g.tryStart(42, 11, at, d) {
		t.Fatal("first pulse must start")
	}
	if g.tryStart(42, 11, at.Add(2*time.Second), d) {
		t.Fatal("a pulse inside the active window must be dropped")
	}
	if !g.tryStart(42, 11, at.Add(d), d) {
		t.Fatal("a pulse at exactly the window end must start")
	}
}

func TestPulseGuard_DroppedTriggerDoesNotExtendWindow(t *testing.T) {
	g := newPulseGuard()
	at := time.Unix(1000, 0)
	d := 5 * time.Second

	g.tryStart(42, 11, at, d)
	g.tryStart(42, 11, at.Add(4*time.Second), d) // dropped
	if !g.tryStart(42, 11, at.Add(d), d) {
		t.Fatal("a dropped trigger must not push the window end out")
	}
}

func TestPulseGuard_IndependentPerOrgAndOutput(t *testing.T) {
	g := newPulseGuard()
	at := time.Unix(1000, 0)
	d := 5 * time.Second

	g.tryStart(42, 11, at, d)
	if !g.tryStart(42, 12, at, d) {
		t.Fatal("a different output must not be guarded")
	}
	if !g.tryStart(43, 11, at, d) {
		t.Fatal("the same output id in a different org must not be guarded")
	}
}

func TestPulseGuard_NonPositiveDurationNeverGuards(t *testing.T) {
	g := newPulseGuard()
	at := time.Unix(1000, 0)

	for i := 0; i < 3; i++ {
		if !g.tryStart(42, 11, at, 0) {
			t.Fatalf("call %d: a zero-length pulse must never be guarded", i)
		}
	}
	if !g.tryStart(42, 11, at, -time.Second) {
		t.Fatal("a negative-length pulse must never be guarded")
	}
	if !g.tryStart(42, 11, at, 5*time.Second) {
		t.Fatal("unguarded calls must not leave a mark behind")
	}
}

func TestPulseGuard_ReleaseClearsOwnMark(t *testing.T) {
	g := newPulseGuard()
	at := time.Unix(1000, 0)
	d := 5 * time.Second

	g.tryStart(42, 11, at, d)
	g.release(42, 11, at, d)
	if !g.tryStart(42, 11, at.Add(time.Second), d) {
		t.Fatal("after release the output must accept a new pulse")
	}
}

func TestPulseGuard_ReleaseLeavesNewerMarkAlone(t *testing.T) {
	g := newPulseGuard()
	at := time.Unix(1000, 0)
	d := 5 * time.Second

	g.tryStart(42, 11, at, d)
	later := at.Add(d)
	g.tryStart(42, 11, later, d) // a newer pulse owns the mark now
	g.release(42, 11, at, d)     // stale release from the first pulse
	if g.tryStart(42, 11, later.Add(time.Second), d) {
		t.Fatal("a stale release must not clear a newer pulse")
	}
}

func TestPulseGuard_ConcurrentStartsHaveOneWinner(t *testing.T) {
	g := newPulseGuard()
	at := time.Unix(1000, 0)
	d := 5 * time.Second

	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if g.tryStart(42, 11, at, d) {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("exactly one concurrent start must win, got %d", wins.Load())
	}
}
