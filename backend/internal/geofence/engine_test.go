package geofence

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
)

// fakeStore records alarm_events writes and serves a fixed device list; either
// call can be made to fail.
type fakeStore struct {
	rows      []storage.AlarmEventRow
	devices   []outputdevice.OutputDevice
	insertErr error
	listErr   error
}

func (s *fakeStore) InsertAlarmEvent(_ context.Context, _ int, ev storage.AlarmEventRow) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.rows = append(s.rows, ev)
	return nil
}

func (s *fakeStore) ListOutputDevicesForLocation(_ context.Context, _, _ int) ([]outputdevice.OutputDevice, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.devices, nil
}

type setCall struct {
	dev      outputdevice.OutputDevice
	on       bool
	offAfter int
}

// fakeDriver records Set calls and can be made to fail. Safe for concurrent use
// (the presence sweeper drives it from a goroutine in production).
type fakeDriver struct {
	mu    sync.Mutex
	calls []setCall
	err   error
}

func (d *fakeDriver) Set(_ context.Context, dev outputdevice.OutputDevice, on bool, offAfter int) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	d.calls = append(d.calls, setCall{dev, on, offAfter})
	return nil
}

func (d *fakeDriver) onCount() int  { return d.count(true) }
func (d *fakeDriver) offCount() int { return d.count(false) }
func (d *fakeDriver) count(on bool) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, c := range d.calls {
		if c.on == on {
			n++
		}
	}
	return n
}

func (d *fakeDriver) first() setCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls[0]
}

// newTestEngine builds an Engine with injected fakes and manual-GC sweepers.
func newTestEngine(cfg Config, s engineStore, d outputDriver) *Engine {
	log := zerolog.New(io.Discard)
	clk := NewFakeClock(time.Unix(0, 0))
	return &Engine{
		cfg:      cfg,
		store:    s,
		driver:   d,
		latch:    newLatch(0, clk),
		presence: newPresence(d, 0, clk, log),
		log:      log,
	}
}

func ptr(i int) *int { return &i }

func read(epc string, rssi int) storage.ResolvedRead {
	return storage.ResolvedRead{AssetID: 7, ScanPointID: 3, LocationID: ptr(9), EPC: epc, RSSI: rssi}
}

func dev(id int, meta map[string]any) outputdevice.OutputDevice {
	return outputdevice.OutputDevice{ID: id, Metadata: meta}
}

func TestEvaluate_EgressFiresOnAndRecordsEvent(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	at := time.Unix(1000, 0)
	e.Evaluate(context.Background(), 42, 555, at, []storage.ResolvedRead{read("EPC1", -50)})

	if d.onCount() != 1 || len(s.rows) != 1 {
		t.Fatalf("expected one ON + one alarm row, got on=%d rows=%d", d.onCount(), len(s.rows))
	}
	row := s.rows[0]
	if row.AssetID != 7 || row.ScanPointID != 3 || row.EPC != "EPC1" || row.RSSI != -50 || row.TagScanID != 555 || !row.FiredAt.Equal(at) {
		t.Fatalf("alarm row fields wrong: %+v", row)
	}
}

func TestEvaluate_SuppressBelowThresholdNoRSSINoLocation(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	noLoc := read("NOLOC", -50)
	noLoc.LocationID = nil
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{
		read("WEAK", -80), read("ZERO", 0), noLoc,
	})
	if d.onCount() != 0 || len(s.rows) != 0 {
		t.Fatalf("weak/no-rssi/no-location reads must not fire, got on=%d rows=%d", d.onCount(), len(s.rows))
	}
}

func TestEvaluate_PerOutputRSSIOverride(t *testing.T) {
	// Global -65 would let -60 fire; a stricter per-output -55 must suppress it.
	strict := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, map[string]any{"rssi_threshold": float64(-55)})}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, strict, d)
	defer e.Stop()
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -60)})
	if d.onCount() != 0 {
		t.Fatalf("per-output -55 override must suppress a -60 read, got %d", d.onCount())
	}
}

func TestEvaluate_EgressLatchPerOutputThenReArm(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	base := time.Unix(1000, 0)
	e.Evaluate(context.Background(), 42, 1, base, []storage.ResolvedRead{read("EPC1", -50)})
	e.Evaluate(context.Background(), 42, 2, base.Add(10*time.Second), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 1 {
		t.Fatalf("repeat within ttl must be latched, got %d", d.onCount())
	}
	e.Evaluate(context.Background(), 42, 3, base.Add(2*time.Minute), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 2 {
		t.Fatalf("re-entry after ttl must re-fire, got %d", d.onCount())
	}
}

func TestEvaluate_EgressPerOutputAgeOutShortensReArm(t *testing.T) {
	// A 10s age-out override should re-arm a 30s gap even though the global TTL is
	// a minute.
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, map[string]any{"age_out_seconds": float64(10)})}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	base := time.Unix(1000, 0)
	e.Evaluate(context.Background(), 42, 1, base, []storage.ResolvedRead{read("EPC1", -50)})
	e.Evaluate(context.Background(), 42, 2, base.Add(30*time.Second), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 2 {
		t.Fatalf("30s gap should re-arm a 10s age-out, got %d", d.onCount())
	}
}

func TestEvaluate_PresenceFiresOnceAutoOffIgnored(t *testing.T) {
	device := dev(11, map[string]any{"mode": "presence", "auto_off_seconds": float64(99)})
	s := &fakeStore{devices: []outputdevice.OutputDevice{device}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	base := time.Unix(1000, 0)
	e.Evaluate(context.Background(), 42, 1, base, []storage.ResolvedRead{read("EPC1", -50)})
	e.Evaluate(context.Background(), 42, 2, base.Add(time.Second), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 1 {
		t.Fatalf("presence should fire ON once for a continuously present tag, got %d", d.onCount())
	}
	// auto_off must be ignored in presence (offAfter == 0).
	if got := d.first().offAfter; got != 0 {
		t.Fatalf("presence ON must pass offAfter=0, got %d", got)
	}
}

func TestEvaluate_BestEffortOnErrors(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}, insertErr: errors.New("db down")}
	d := &fakeDriver{err: errors.New("device offline")}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()
	// Must not panic even though both side effects error.
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -50)})
}

func TestEvaluate_ListErrorIsBestEffort(t *testing.T) {
	s := &fakeStore{listErr: errors.New("lookup failed")}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 0 || len(s.rows) != 0 {
		t.Fatal("a device lookup error must suppress firing without panicking")
	}
}

func TestConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("GEOFENCE_RSSI_THRESHOLD", "")
	t.Setenv("GEOFENCE_LATCH_TTL", "")
	t.Setenv("GEOFENCE_SWEEP_INTERVAL", "")
	c := ConfigFromEnv()
	if c != DefaultConfig() {
		t.Fatalf("empty env must yield defaults, got %+v", c)
	}
}

func TestConfigFromEnv_Overrides(t *testing.T) {
	t.Setenv("GEOFENCE_RSSI_THRESHOLD", "-55")
	t.Setenv("GEOFENCE_LATCH_TTL", "30s")
	t.Setenv("GEOFENCE_SWEEP_INTERVAL", "2m")
	c := ConfigFromEnv()
	if c.RSSIThreshold != -55 || c.LatchTTL != 30*time.Second || c.SweepInterval != 2*time.Minute {
		t.Fatalf("env overrides not applied: %+v", c)
	}

	// Malformed values fall back to defaults.
	t.Setenv("GEOFENCE_RSSI_THRESHOLD", "nope")
	t.Setenv("GEOFENCE_LATCH_TTL", "nope")
	if c := ConfigFromEnv(); c.RSSIThreshold != DefaultConfig().RSSIThreshold || c.LatchTTL != DefaultConfig().LatchTTL {
		t.Fatalf("malformed env must fall back to defaults: %+v", c)
	}
}
