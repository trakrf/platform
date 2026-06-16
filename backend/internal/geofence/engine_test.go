package geofence

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
)

// fakeStore records alarm_events writes and serves a fixed device list; either
// call can be made to fail.
type fakeStore struct {
	rows        []storage.AlarmEventRow
	devices     []outputdevice.OutputDevice
	orgDefaults organization.GeofenceDefaults
	insertErr   error
	listErr     error
	defaultsErr error
}

func (s *fakeStore) GetOrgGeofenceDefaults(_ context.Context, _ int) (organization.GeofenceDefaults, error) {
	if s.defaultsErr != nil {
		return organization.GeofenceDefaults{}, s.defaultsErr
	}
	return s.orgDefaults, nil
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
		cfg:          cfg,
		store:        s,
		driver:       d,
		latch:        newLatch(0, clk),
		presence:     newPresence(d, log),
		startupGrace: cfg.StartupGrace,
		log:          log,
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

func TestEvaluate_OrgDefaultRSSIAppliesWhenDeviceUnset(t *testing.T) {
	// Code default -65 would let a -60 read fire; a stricter org default -55 (with
	// no per-output override) must suppress it.
	s := &fakeStore{
		devices:     []outputdevice.OutputDevice{dev(11, nil)},
		orgDefaults: organization.GeofenceDefaults{RSSIThreshold: ptr(-55)},
	}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -60)})
	if d.onCount() != 0 {
		t.Fatalf("org-default -55 must suppress a -60 read on an un-overridden device, got %d", d.onCount())
	}
}

func TestEvaluate_DeviceOverridesOrgDefault(t *testing.T) {
	// Org default -55 would suppress -60, but a per-output override of -65 must win,
	// so the -60 read fires.
	s := &fakeStore{
		devices:     []outputdevice.OutputDevice{dev(11, map[string]any{"rssi_threshold": float64(-65)})},
		orgDefaults: organization.GeofenceDefaults{RSSIThreshold: ptr(-55)},
	}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -60)})
	if d.onCount() != 1 {
		t.Fatalf("per-output -65 must override org-default -55 and fire a -60 read, got %d", d.onCount())
	}
}

func TestEvaluate_OrgDefaultAgeOutShortensReArm(t *testing.T) {
	// An org-default 10s age-out (no per-output override) should re-arm a 30s gap
	// even though the code-default latch TTL is a minute.
	s := &fakeStore{
		devices:     []outputdevice.OutputDevice{dev(11, nil)},
		orgDefaults: organization.GeofenceDefaults{AgeOutSeconds: ptr(10)},
	}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute}, s, d)
	defer e.Stop()

	base := time.Unix(1000, 0)
	e.Evaluate(context.Background(), 42, 1, base, []storage.ResolvedRead{read("EPC1", -50)})
	e.Evaluate(context.Background(), 42, 2, base.Add(30*time.Second), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 2 {
		t.Fatalf("30s gap should re-arm a 10s org-default age-out, got %d", d.onCount())
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

// --- TRA-991: startup grace window ---
//
// On a cold start the engine has no prior membership state, so every tag already
// in the read zone would otherwise register as a fresh entry and fire (and
// actuate the Shelly). The startup grace window hydrates those tags as an
// "already inside" baseline — it still records them in the latch/presence state —
// but suppresses the ON edge until the window, opened on the first evaluated
// read, has elapsed.

func TestEvaluate_StartupGraceSuppressesInZoneEgress(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute, StartupGrace: 30 * time.Second}, s, d)
	defer e.Stop()

	base := time.Unix(1000, 0)
	// A tag present at startup, read repeatedly during the grace window, never fires.
	e.Evaluate(context.Background(), 42, 1, base, []storage.ResolvedRead{read("EPC1", -50)})
	e.Evaluate(context.Background(), 42, 2, base.Add(5*time.Second), []storage.ResolvedRead{read("EPC1", -50)})
	e.Evaluate(context.Background(), 42, 3, base.Add(29*time.Second), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 0 || len(s.rows) != 0 {
		t.Fatalf("in-zone tag must not fire during the startup grace window, got on=%d rows=%d", d.onCount(), len(s.rows))
	}
}

func TestEvaluate_StartupGraceBaselineStaysSilentNewEntryFires(t *testing.T) {
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute, StartupGrace: 30 * time.Second}, s, d)
	defer e.Stop()

	base := time.Unix(1000, 0)
	// Seed EPC1 as baseline during grace (suppressed, but recorded in the latch).
	e.Evaluate(context.Background(), 42, 1, base, []storage.ResolvedRead{read("EPC1", -50)})
	// After the window closes, the still-present baseline tag stays latched (no fire).
	e.Evaluate(context.Background(), 42, 2, base.Add(31*time.Second), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 0 {
		t.Fatalf("baseline tag still in zone after grace must stay latched, got on=%d", d.onCount())
	}
	// ...but a genuinely new tag arriving after the window fires.
	e.Evaluate(context.Background(), 42, 3, base.Add(32*time.Second), []storage.ResolvedRead{read("EPC2", -50)})
	if d.onCount() != 1 || len(s.rows) != 1 {
		t.Fatalf("a new entry after the grace window must fire, got on=%d rows=%d", d.onCount(), len(s.rows))
	}
}

func TestEvaluate_StartupGraceSeedsPresenceBaseline(t *testing.T) {
	device := dev(11, map[string]any{"mode": "presence"})
	s := &fakeStore{devices: []outputdevice.OutputDevice{device}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute, StartupGrace: 30 * time.Second}, s, d)
	defer e.Stop()

	base := time.Unix(1000, 0)
	e.Evaluate(context.Background(), 42, 1, base, []storage.ResolvedRead{read("EPC1", -50)})
	e.Evaluate(context.Background(), 42, 2, base.Add(5*time.Second), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 0 {
		t.Fatalf("presence in-zone tag must not fire ON during the grace window, got %d", d.onCount())
	}
	// The tag was seeded as present, so after grace a continued read is not a fresh
	// 0->1 edge and still does not fire.
	e.Evaluate(context.Background(), 42, 3, base.Add(31*time.Second), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 0 {
		t.Fatalf("presence baseline tag must stay silent after grace, got %d", d.onCount())
	}
}

func TestEvaluate_StartupGraceDisabledFiresImmediately(t *testing.T) {
	// StartupGrace == 0 disables the window: an in-zone tag fires on first sight.
	s := &fakeStore{devices: []outputdevice.OutputDevice{dev(11, nil)}}
	d := &fakeDriver{}
	e := newTestEngine(Config{RSSIThreshold: -65, LatchTTL: time.Minute, StartupGrace: 0}, s, d)
	defer e.Stop()
	e.Evaluate(context.Background(), 42, 1, time.Unix(1000, 0), []storage.ResolvedRead{read("EPC1", -50)})
	if d.onCount() != 1 {
		t.Fatalf("a zero grace window must not suppress firing, got %d", d.onCount())
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
	t.Setenv("GEOFENCE_SWEEP_INTERVAL", "")
	c := ConfigFromEnv()
	if c != DefaultConfig() {
		t.Fatalf("empty env must yield defaults, got %+v", c)
	}
}

func TestConfigFromEnv_StartupGrace(t *testing.T) {
	t.Setenv("GEOFENCE_STARTUP_GRACE", "45s")
	if c := ConfigFromEnv(); c.StartupGrace != 45*time.Second {
		t.Fatalf("GEOFENCE_STARTUP_GRACE not applied, got %v", c.StartupGrace)
	}
	// Malformed falls back to the default.
	t.Setenv("GEOFENCE_STARTUP_GRACE", "nope")
	if c := ConfigFromEnv(); c.StartupGrace != DefaultConfig().StartupGrace {
		t.Fatalf("malformed GEOFENCE_STARTUP_GRACE must fall back to default, got %v", c.StartupGrace)
	}
	// Explicit zero disables the window (distinct from unset, which keeps the default).
	t.Setenv("GEOFENCE_STARTUP_GRACE", "0s")
	if c := ConfigFromEnv(); c.StartupGrace != 0 {
		t.Fatalf("GEOFENCE_STARTUP_GRACE=0s must disable the window, got %v", c.StartupGrace)
	}
}

func TestConfigFromEnv_SweepIntervalOnly(t *testing.T) {
	// RSSI threshold and latch TTL were retired as env knobs (TRA-955): they are
	// system/code defaults now, tuned per-org/per-output at runtime. Only the
	// engine-global sweep interval remains env-configurable.
	t.Setenv("GEOFENCE_RSSI_THRESHOLD", "-10") // retired: must be ignored
	t.Setenv("GEOFENCE_LATCH_TTL", "1s")       // retired: must be ignored
	t.Setenv("GEOFENCE_SWEEP_INTERVAL", "30s")
	c := ConfigFromEnv()
	if c.RSSIThreshold != DefaultConfig().RSSIThreshold {
		t.Fatalf("GEOFENCE_RSSI_THRESHOLD should be retired, got %d", c.RSSIThreshold)
	}
	if c.LatchTTL != DefaultConfig().LatchTTL {
		t.Fatalf("GEOFENCE_LATCH_TTL should be retired, got %v", c.LatchTTL)
	}
	if c.SweepInterval != 30*time.Second {
		t.Fatalf("sweep interval not applied: %v", c.SweepInterval)
	}

	// Malformed sweep interval falls back to the default.
	t.Setenv("GEOFENCE_SWEEP_INTERVAL", "nope")
	if c := ConfigFromEnv(); c.SweepInterval != DefaultConfig().SweepInterval {
		t.Fatalf("malformed sweep interval must fall back to default: %v", c.SweepInterval)
	}
}
