package geofence

import (
	"context"
	"errors"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
)

// pulse5s is egress metadata with a 5s device-side auto-off.
var pulse5s = map[string]any{"auto_off_seconds": float64(5)}

// pulseActiveDrops reads the global pulse_active suppression counter. Tests
// assert deltas, since the counter is shared by every test in the package.
func pulseActiveDrops(t *testing.T) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := metricSuppressed.WithLabelValues("pulse_active").Write(m); err != nil {
		t.Fatalf("read pulse_active counter: %v", err)
	}
	return m.GetCounter().GetValue()
}

func TestEvaluate_EgressDropsOverlappingPulse(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, pulse5s)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	before := pulseActiveDrops(t)
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{
		read("EPC1", -50), read("EPC2", -50),
	})

	if d.onCount() != 1 || len(s.rows) != 1 {
		t.Fatalf("two tags together must give one ON + one alarm row, got on=%d rows=%d", d.onCount(), len(s.rows))
	}
	if s.rows[0].EPC != "EPC1" {
		t.Fatalf("the first tag must be the one that fires, got %s", s.rows[0].EPC)
	}
	if got := pulseActiveDrops(t) - before; got != 1 {
		t.Fatalf("expected one pulse_active drop, got %v", got)
	}
}

func TestEvaluate_EgressFiresAgainAfterPulseEnds(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, pulse5s)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	at := time.Unix(1000, 0)
	e.Evaluate(context.Background(), 42, 1, at, []storage.ResolvedRead{read("EPC1", -50)})
	e.Evaluate(context.Background(), 42, 2, at.Add(2*time.Second), []storage.ResolvedRead{read("EPC2", -50)})
	if d.onCount() != 1 {
		t.Fatalf("a tag inside the 5s pulse must be dropped, got on=%d", d.onCount())
	}
	e.Evaluate(context.Background(), 42, 3, at.Add(5*time.Second), []storage.ResolvedRead{read("EPC3", -50)})
	if d.onCount() != 2 || len(s.rows) != 2 {
		t.Fatalf("a tag once the pulse has ended must fire, got on=%d rows=%d", d.onCount(), len(s.rows))
	}
}

func TestEvaluate_EgressPulseGuardIsPerOutput(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, pulse5s), dev(12, pulse5s)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{
		read("EPC1", -50), read("EPC2", -50),
	})

	if d.onCount() != 2 {
		t.Fatalf("each output must pulse once, got on=%d", d.onCount())
	}
	seen := map[int]int{}
	for _, c := range d.calls {
		seen[c.dev.ID]++
	}
	if seen[11] != 1 || seen[12] != 1 {
		t.Fatalf("expected one ON per output, got %v", seen)
	}
}

func TestEvaluate_EgressWithoutAutoOffIsNotGuarded(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	before := pulseActiveDrops(t)
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{
		read("EPC1", -50), read("EPC2", -50),
	})

	if d.onCount() != 2 || len(s.rows) != 2 {
		t.Fatalf("with no auto-off both tags must fire as before, got on=%d rows=%d", d.onCount(), len(s.rows))
	}
	if got := pulseActiveDrops(t) - before; got != 0 {
		t.Fatalf("no pulse_active drops expected without auto-off, got %v", got)
	}
}

func TestEvaluate_EgressFailedDriveDoesNotBlockNextTag(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, pulse5s)}}
	d := &fakeDriver{err: errors.New("device offline")}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	at := time.Unix(1000, 0)
	e.Evaluate(context.Background(), 42, 1, at, []storage.ResolvedRead{read("EPC1", -50)})

	d.mu.Lock()
	d.err = nil
	d.mu.Unlock()
	e.Evaluate(context.Background(), 42, 2, at.Add(time.Second), []storage.ResolvedRead{read("EPC2", -50)})

	if d.onCount() != 1 {
		t.Fatalf("a failed drive must not hold the output busy, got on=%d", d.onCount())
	}
}
