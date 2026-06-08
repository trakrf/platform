package geofence

import (
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

// recordedTimer is a fake timer the test fires manually. Stop() reports whether
// it was still pending (matching *time.Timer semantics) and prevents a later
// manual fire from running the callback.
type recordedTimer struct {
	d       time.Duration
	f       func()
	stopped bool
}

func (t *recordedTimer) Stop() bool {
	was := t.stopped
	t.stopped = true
	return !was
}

// fire runs the callback if the timer is still pending (mirrors a real timer:
// a stopped timer never fires).
func (t *recordedTimer) fire() {
	if !t.stopped {
		t.stopped = true
		t.f()
	}
}

// newTestPresence builds a presence tracker whose timers are captured for manual
// firing instead of running on the wall clock.
func newTestPresence(driver outputDriver) (*presence, *[]*recordedTimer) {
	p := newPresence(driver, zerolog.New(io.Discard))
	timers := &[]*recordedTimer{}
	p.afterFunc = func(d time.Duration, f func()) timerHandle {
		rt := &recordedTimer{d: d, f: f}
		*timers = append(*timers, rt)
		return rt
	}
	return p, timers
}

func TestPresence_FirstObserveFiresOnReObserveDoesNot(t *testing.T) {
	p, _ := newTestPresence(&fakeDriver{})
	defer p.Close()
	dev := outputdevice.OutputDevice{ID: 5}

	if !p.observe(1, dev, time.Minute, "EPC") {
		t.Fatal("first observe must report the ON edge")
	}
	if p.observe(1, dev, time.Minute, "EPC") {
		t.Fatal("re-observe while present must not report a new ON edge")
	}
}

func TestPresence_TimerFiresOffAfterAgeOut(t *testing.T) {
	d := &fakeDriver{}
	p, timers := newTestPresence(d)
	defer p.Close()
	dev := outputdevice.OutputDevice{ID: 5}

	p.observe(1, dev, 6*time.Second, "EPC")
	if len(*timers) != 1 || (*timers)[0].d != 6*time.Second {
		t.Fatalf("expected one timer armed at 6s, got %+v", *timers)
	}

	(*timers)[0].fire() // age-out elapsed with no further read
	if d.offCount() != 1 {
		t.Fatalf("age-out must drive exactly one OFF, got %d", d.offCount())
	}

	// After OFF the output is forgotten: the next read is a fresh ON edge.
	if !p.observe(1, dev, 6*time.Second, "EPC") {
		t.Fatal("observe after OFF must report a new ON edge")
	}
}

func TestPresence_ReadResetsTimerPreventingPrematureOff(t *testing.T) {
	d := &fakeDriver{}
	p, timers := newTestPresence(d)
	defer p.Close()
	dev := outputdevice.OutputDevice{ID: 5}

	p.observe(1, dev, 6*time.Second, "EPC") // arms timer[0]
	p.observe(1, dev, 6*time.Second, "EPC") // a read within age-out: stops timer[0], arms timer[1]

	// The stale timer[0] firing late (lost the race) must not drive OFF.
	(*timers)[0].fire()
	if d.offCount() != 0 {
		t.Fatalf("a reset (stale) timer must not fire OFF, got %d", d.offCount())
	}

	// The current timer firing does drive OFF.
	(*timers)[1].fire()
	if d.offCount() != 1 {
		t.Fatalf("current timer must drive OFF, got %d", d.offCount())
	}
}

func TestPresence_TwoMembersOffOnlyAfterLastGoesQuiet(t *testing.T) {
	d := &fakeDriver{}
	p, timers := newTestPresence(d)
	defer p.Close()
	dev := outputdevice.OutputDevice{ID: 5}

	if !p.observe(1, dev, 6*time.Second, "A") {
		t.Fatal("first member is the ON edge")
	}
	if p.observe(1, dev, 6*time.Second, "B") {
		t.Fatal("second member must not be a new ON edge")
	}
	// Each read reset the single per-output timer; only the latest is live.
	(*timers)[0].fire() // A's reset timer — stale
	if d.offCount() != 0 {
		t.Fatalf("output must stay on while a member is still being read, got %d off", d.offCount())
	}
	(*timers)[len(*timers)-1].fire() // last armed timer: no read for age-out
	if d.offCount() != 1 {
		t.Fatalf("OFF once the last member goes quiet, got %d", d.offCount())
	}
}

func TestPresence_CloseStopsTimersAndBlocksObserve(t *testing.T) {
	d := &fakeDriver{}
	p, timers := newTestPresence(d)
	dev := outputdevice.OutputDevice{ID: 5}

	p.observe(1, dev, 6*time.Second, "EPC")
	p.Close()
	// A timer that fires after Close was stopped, so it no-ops.
	(*timers)[0].fire()
	if d.offCount() != 0 {
		t.Fatalf("timers stopped on Close must not fire, got %d", d.offCount())
	}
	if p.observe(1, dev, 6*time.Second, "EPC") {
		t.Fatal("observe after Close must be a no-op (return false)")
	}
}
