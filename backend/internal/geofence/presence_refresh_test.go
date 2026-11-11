package geofence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/readerrpc"
	"github.com/trakrf/platform/backend/internal/storage"
)

// TRA-1138: no output latches. A presence ON carries the on-time the reader
// enforces, and the engine keeps re-sending it while a tag stays in view, so a
// wanted alarm holds while a lost connection stops it within the on-time.

// flakyDriver fails the ONs it is told to, in call order.
type flakyDriver struct {
	mu    sync.Mutex
	calls []setCall
	fail  map[int]bool // 0-based index of ON calls that fail
	ons   int
}

func (d *flakyDriver) Set(_ context.Context, dev outputdevice.OutputDevice, on bool, offAfter int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, setCall{dev, on, offAfter})
	if on {
		i := d.ons
		d.ons++
		if d.fail[i] {
			return errors.New("broker unreachable")
		}
	}
	return nil
}

func (d *flakyDriver) onCalls() []setCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []setCall
	for _, c := range d.calls {
		if c.on {
			out = append(out, c)
		}
	}
	return out
}

func presenceEngine(t *testing.T, d outputDriver, s *fakeStore) *Engine {
	t.Helper()
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	t.Cleanup(e.Stop)
	return e
}

func presenceStore() *fakeStore {
	return &fakeStore{devices: []outputdevice.OutputDevice{dev(11, map[string]any{"mode": "presence"})}}
}

func evalAt(e *Engine, id int64, at time.Time) {
	e.Evaluate(context.Background(), 42, id, at, []storage.ResolvedRead{read("EPC1", -50)})
}

var wantOnSec = int(readerrpc.GpoDefaultOn / time.Second)

func TestEvaluate_PresenceOnCarriesTheOnTime(t *testing.T) {
	d := &fakeDriver{}
	e := presenceEngine(t, d, presenceStore())

	evalAt(e, 1, time.Unix(1000, 0))
	if d.onCount() != 1 {
		t.Fatalf("on = %d, want 1", d.onCount())
	}
	if got := d.first().offAfter; got != wantOnSec {
		t.Errorf("presence ON offAfter = %d, want %d (readerrpc.GpoDefaultOn)", got, wantOnSec)
	}
}

// Reads every 5 s for 21 s: ONs at 0, 10 and 20 s, every one with the on-time.
// Only the first is a fire; a refresh writes no alarm_events row.
func TestEvaluate_PresenceRefreshesWhileATagStaysInView(t *testing.T) {
	d := &fakeDriver{}
	s := presenceStore()
	e := presenceEngine(t, d, s)

	base := time.Unix(1000, 0)
	for i, sec := range []int{0, 5, 10, 15, 20} {
		evalAt(e, int64(i+1), base.Add(time.Duration(sec)*time.Second))
	}
	if d.onCount() != 3 {
		t.Fatalf("on = %d, want 3 (at 0, 10 and 20 s)", d.onCount())
	}
	for _, c := range d.calls {
		if c.on && c.offAfter != wantOnSec {
			t.Errorf("an ON carried offAfter %d, want %d", c.offAfter, wantOnSec)
		}
	}
	if d.offCount() != 0 {
		t.Errorf("off = %d while the tag stayed in view, want 0", d.offCount())
	}
	if len(s.rows) != 1 {
		t.Errorf("alarm_events rows = %d, want 1 (refreshes are not fires)", len(s.rows))
	}
}

// The refresh interval must leave room for lost refreshes inside the on-time:
// two refreshes in a row can be lost before the reader lets the output go.
func TestPresenceRefreshInterval(t *testing.T) {
	if presenceRefreshEvery != 10*time.Second {
		t.Errorf("presenceRefreshEvery = %v, want 10s", presenceRefreshEvery)
	}
	if 3*presenceRefreshEvery > readerrpc.GpoDefaultOn {
		t.Errorf("refresh %v x3 exceeds the %v on-time", presenceRefreshEvery, readerrpc.GpoDefaultOn)
	}
}

// A failed ON (a broker blip) is retried on the next read instead of leaving
// the alarm silent until the tag leaves and comes back.
func TestEvaluate_PresenceRetriesAFailedOn(t *testing.T) {
	d := &flakyDriver{fail: map[int]bool{0: true}}
	e := presenceEngine(t, d, presenceStore())

	base := time.Unix(1000, 0)
	evalAt(e, 1, base)
	evalAt(e, 2, base.Add(time.Second))
	if got := len(d.onCalls()); got != 2 {
		t.Fatalf("ON attempts = %d, want 2 (the failed first ON retried on the next read)", got)
	}
	// Now held: the next read inside 10 s sends nothing.
	evalAt(e, 3, base.Add(2*time.Second))
	if got := len(d.onCalls()); got != 2 {
		t.Errorf("ON attempts = %d after a held read, want 2", got)
	}
}

// A failed refresh is retried on the next read too, so one lost publish does
// not cost a whole refresh interval.
func TestEvaluate_PresenceRetriesAFailedRefresh(t *testing.T) {
	d := &flakyDriver{fail: map[int]bool{1: true}}
	e := presenceEngine(t, d, presenceStore())

	base := time.Unix(1000, 0)
	evalAt(e, 1, base)                     // ON
	evalAt(e, 2, base.Add(10*time.Second)) // refresh fails
	evalAt(e, 3, base.Add(11*time.Second)) // retried
	evalAt(e, 4, base.Add(12*time.Second)) // held
	if got := len(d.onCalls()); got != 3 {
		t.Errorf("ON attempts = %d, want 3", got)
	}
}

// Startup grace is unchanged: a tag already in the zone at a restart is a
// baseline, so it is never refreshed into an alarm either.
func TestEvaluate_PresenceGraceBaselineIsNeverRefreshedOn(t *testing.T) {
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute, StartupGrace: 30 * time.Second}, presenceStore(), d)
	t.Cleanup(e.Stop)

	base := time.Unix(1000, 0)
	for i, sec := range []int{0, 10, 31, 41, 51} {
		evalAt(e, int64(i+1), base.Add(time.Duration(sec)*time.Second))
	}
	if d.onCount() != 0 {
		t.Errorf("on = %d, want 0 for a grace baseline tag", d.onCount())
	}
}
