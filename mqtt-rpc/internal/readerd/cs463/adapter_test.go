package cs463

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// fakeOps is an injectable readerOps for adapter tests. It records calls and
// serves scripted profiles. getProfile may return a different profile on the
// 2nd+ call (to exercise the antenna-wipe verify guard).
type fakeOps struct {
	session  string
	holderIP string
	loginErr error

	profiles []Profile // returned in order across GetActiveProfile calls; last repeats
	profErr  error

	setErr         error
	enableEr       error
	loginServletEr error

	// recorded
	loggedOut    bool
	forced       bool
	getCalls     int
	setProfileID string
	setAntCount  int
	setPorts     []int
	setPowers    map[int]float64
	setFields    map[string]string
	enableSeq    []bool

	// new fields for GetOperProfile / SetOperProfile
	events    map[string]EntityRow
	logics    map[string]EntityRow
	modEvents []url.Values // recorded ModEvent calls

	// GPO: guarded because a pulse releases from its own goroutine.
	mu       sync.Mutex
	gpoCalls []gpoCall
	gpoErr   error

	// callOrder records method names in the order they were invoked, used to
	// assert LoginServlet runs before SetProfilePower.
	callOrder []string
}

func (f *fakeOps) Login(ctx context.Context) (session, holderIP string, err error) {
	f.callOrder = append(f.callOrder, "Login")
	if f.loginErr != nil {
		return "", "", f.loginErr
	}
	if f.holderIP != "" {
		return "", f.holderIP, nil
	}
	s := f.session
	if s == "" {
		s = "sess1"
	}
	return s, "", nil
}

func (f *fakeOps) Logout(ctx context.Context, session string) error {
	f.callOrder = append(f.callOrder, "Logout")
	f.loggedOut = true
	return nil
}

func (f *fakeOps) ForceLogout(ctx context.Context) error {
	f.forced = true
	f.holderIP = "" // a forced logout frees the held session
	return nil
}

func (f *fakeOps) GetActiveProfile(ctx context.Context, session string) (Profile, error) {
	f.callOrder = append(f.callOrder, "GetActiveProfile")
	if f.profErr != nil {
		return Profile{}, f.profErr
	}
	i := f.getCalls
	f.getCalls++
	if i >= len(f.profiles) {
		i = len(f.profiles) - 1
	}
	return f.profiles[i], nil
}

func (f *fakeOps) LoginServlet(ctx context.Context) error {
	f.callOrder = append(f.callOrder, "LoginServlet")
	return f.loginServletEr
}

func (f *fakeOps) LogoutServlet(ctx context.Context) error {
	f.callOrder = append(f.callOrder, "LogoutServlet")
	return nil
}

func (f *fakeOps) SetProfilePower(ctx context.Context, profileID string, antennaCount int, enabledPorts []int, powers map[int]float64, profileFields map[string]string) error {
	f.callOrder = append(f.callOrder, "SetProfilePower")
	f.setProfileID = profileID
	f.setAntCount = antennaCount
	f.setPorts = enabledPorts
	f.setPowers = powers
	f.setFields = profileFields
	return f.setErr
}

func (f *fakeOps) CreateProfile(ctx context.Context, session, profileID string, txPowerDBm float64) error {
	f.callOrder = append(f.callOrder, "CreateProfile")
	return nil
}

func (f *fakeOps) EnableEvent(ctx context.Context, session, eventID string, enable bool) error {
	if enable {
		f.callOrder = append(f.callOrder, "EnableEvent(true)")
	} else {
		f.callOrder = append(f.callOrder, "EnableEvent(false)")
	}
	f.enableSeq = append(f.enableSeq, enable)
	return f.enableEr
}

func (f *fakeOps) ListEvent(ctx context.Context, session string) (map[string]EntityRow, error) {
	return f.events, nil
}

func (f *fakeOps) DirectIOOutput(ctx context.Context, port int, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gpoCalls = append(f.gpoCalls, gpoCall{port: port, on: on})
	return f.gpoErr
}

type gpoCall struct {
	port int
	on   bool
}

func (f *fakeOps) ListTriggeringLogic(ctx context.Context, session string) (map[string]EntityRow, error) {
	return f.logics, nil
}

func (f *fakeOps) ModEvent(ctx context.Context, session string, p url.Values) error {
	f.callOrder = append(f.callOrder, "ModEvent")
	f.modEvents = append(f.modEvents, p)
	return nil
}

func profile(id, antennaPort string, powers map[int]float64) Profile {
	attrs := map[string]string{"profile_id": id, "antenna_port": antennaPort, "active": "true"}
	return Profile{ID: id, Attrs: attrs, Powers: powers}
}

func newAdapter(f *fakeOps) *Adapter {
	return NewAdapter(f, AdapterConfig{AntennaCount: 4, EventID: "evt1"})
}

func TestGetCapabilities(t *testing.T) {
	a := newAdapter(&fakeOps{})
	caps, err := a.GetCapabilities(context.Background())
	if err != nil {
		t.Fatalf("GetCapabilities: %v", err)
	}
	if caps.ContractVersion != readerrpc.ContractVersion {
		t.Errorf("contract version = %d, want %d", caps.ContractVersion, readerrpc.ContractVersion)
	}
	if caps.ReaderModel != "csl_cs463" {
		t.Errorf("reader model = %q", caps.ReaderModel)
	}
	if caps.Antennas != 4 {
		t.Errorf("antennas = %d, want 4", caps.Antennas)
	}
	if caps.TxPower.MinDBm != 10.0 || caps.TxPower.MaxDBm != 31.5 || !caps.TxPower.PerAntenna {
		t.Errorf("tx power cap = %+v", caps.TxPower)
	}
	wantSupports := []string{
		readerrpc.MethodGetCapabilities,
		readerrpc.MethodGetOperProfile,
		readerrpc.MethodSetOperProfile,
		readerrpc.MethodGetStatus,
		readerrpc.MethodGpoSet,
	}
	if !reflect.DeepEqual(caps.Supports, wantSupports) {
		t.Errorf("supports = %v, want %v", caps.Supports, wantSupports)
	}
	wantUnsupported := []string{
		readerrpc.MethodScanStart,
		readerrpc.MethodScanStop,
		readerrpc.MethodReboot,
	}
	if !reflect.DeepEqual(caps.Unsupported, wantUnsupported) {
		t.Errorf("unsupported = %v, want %v", caps.Unsupported, wantUnsupported)
	}
}

func TestGetOperProfile_MapsAntennasAndKnobs(t *testing.T) {
	prof := Profile{
		ID:     "TrakRF",
		Attrs:  map[string]string{"profile_id": "TrakRF", "antenna_port": "1,2", "active": "true", "dwellTime1": "500"},
		Powers: map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0},
	}
	f := &fakeOps{
		profiles: []Profile{prof},
		events:   map[string]EntityRow{NameEvent: {"duplicateEliminationWindow": "500", "antennaDifferentiation": "true"}},
		logics:   map[string]EntityRow{NameTrigger: {"logic": "-80"}},
	}
	a := newAdapter(f)
	cfg, err := a.GetOperProfile(context.Background(), false)
	if err != nil {
		t.Fatalf("GetOperProfile: %v", err)
	}
	want := []readerrpc.AntennaConfig{
		{Antenna: 1, Enabled: true, PowerDBm: 30.0},
		{Antenna: 2, Enabled: true, PowerDBm: 22.5},
		{Antenna: 3, Enabled: false, PowerDBm: 0.0},
		{Antenna: 4, Enabled: false, PowerDBm: 0.0},
	}
	if !reflect.DeepEqual(cfg.Antennas, want) {
		t.Errorf("antennas = %+v, want %+v", cfg.Antennas, want)
	}
	if cfg.DwellMs == nil || *cfg.DwellMs != 500 {
		t.Errorf("dwell = %v, want 500", cfg.DwellMs)
	}
	if cfg.DedupWindowMs == nil || *cfg.DedupWindowMs != 500 {
		t.Errorf("dedup = %v, want 500", cfg.DedupWindowMs)
	}
	if cfg.RSSIGateDBm == nil || *cfg.RSSIGateDBm != -80 {
		t.Errorf("rssi gate = %v, want -80", cfg.RSSIGateDBm)
	}
	if cfg.AntennaDifferentiation == nil || !*cfg.AntennaDifferentiation {
		t.Errorf("antenna differentiation = %v, want true", cfg.AntennaDifferentiation)
	}
	if !f.loggedOut {
		t.Error("expected logout")
	}
}

func TestGetOperProfile_BusyWithoutForce(t *testing.T) {
	f := &fakeOps{holderIP: "192.168.50.203", profiles: []Profile{profile("TrakRF", "1,2", nil)}}
	a := newAdapter(f)
	_, err := a.GetOperProfile(context.Background(), false)
	var be *readerrpc.BusyError
	if !errors.As(err, &be) {
		t.Fatalf("want *readerrpc.BusyError, got %v", err)
	}
	if be.HeldBy != "192.168.50.203" {
		t.Errorf("held_by = %q", be.HeldBy)
	}
	if f.forced {
		t.Error("must NOT force without force flag")
	}
}

func TestGetOperProfile_ForceClearsBusy(t *testing.T) {
	f := &fakeOps{holderIP: "192.168.50.203", profiles: []Profile{profile("TrakRF", "1,2", map[int]float64{1: 30})}}
	a := newAdapter(f)
	_, err := a.GetOperProfile(context.Background(), true)
	if err != nil {
		t.Fatalf("GetOperProfile(force): %v", err)
	}
	if !f.forced {
		t.Error("expected ForceLogout on force")
	}
}

func TestSetOperProfile_PushesEnablement(t *testing.T) {
	// active: ports 1,2 enabled; request enables 1,2,3 and sets powers.
	before := profile("TrakRF", "1,2", map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0})
	after := profile("TrakRF", "1,2,3", map[int]float64{1: 25.0, 2: 22.5, 3: 28.0, 4: 0.0})
	f := &fakeOps{profiles: []Profile{before, after}}
	a := newAdapter(f)

	res, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{
		Antennas: []readerrpc.AntennaConfig{
			{Antenna: 1, Enabled: true, PowerDBm: 25.0},
			{Antenna: 2, Enabled: true, PowerDBm: 22.5},
			{Antenna: 3, Enabled: true, PowerDBm: 28.0},
		},
	}, false)
	if err != nil {
		t.Fatalf("SetOperProfile: %v", err)
	}
	if res.Applied != readerrpc.AppliedPendingReload {
		t.Errorf("applied = %q", res.Applied)
	}
	if !reflect.DeepEqual(f.setPorts, []int{1, 2, 3}) {
		t.Errorf("enabled ports = %v, want [1 2 3]", f.setPorts)
	}
	wantPowers := map[int]float64{1: 25.0, 2: 22.5, 3: 28.0, 4: 0.0}
	if !reflect.DeepEqual(f.setPowers, wantPowers) {
		t.Errorf("powers = %v, want %v", f.setPowers, wantPowers)
	}
	if !reflect.DeepEqual(f.enableSeq, []bool{false, true}) {
		t.Errorf("re-arm = %v, want [false true]", f.enableSeq)
	}
}

func TestSetOperProfile_DisableAntenna(t *testing.T) {
	before := profile("TrakRF", "1,2", map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0})
	after := profile("TrakRF", "1", map[int]float64{1: 30.0})
	f := &fakeOps{profiles: []Profile{before, after}}
	a := newAdapter(f)

	if _, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{
		Antennas: []readerrpc.AntennaConfig{
			{Antenna: 1, Enabled: true, PowerDBm: 30.0},
			{Antenna: 2, Enabled: false, PowerDBm: 22.5},
		},
	}, false); err != nil {
		t.Fatalf("SetOperProfile: %v", err)
	}
	if !reflect.DeepEqual(f.setPorts, []int{1}) {
		t.Errorf("enabled ports = %v, want [1]", f.setPorts)
	}
}

func TestSetOperProfile_VerifyMismatchFails(t *testing.T) {
	before := profile("TrakRF", "1,2", map[int]float64{1: 30.0, 2: 22.5})
	after := profile("TrakRF", "", map[int]float64{1: 25.0}) // servlet wiped antenna_port
	f := &fakeOps{profiles: []Profile{before, after}}
	a := newAdapter(f)
	_, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{
		Antennas: []readerrpc.AntennaConfig{{Antenna: 1, Enabled: true, PowerDBm: 25.0}, {Antenna: 2, Enabled: true, PowerDBm: 22.5}},
	}, false)
	if err == nil {
		t.Fatal("expected verify-mismatch error")
	}
	if f.setProfileID == "" {
		t.Error("guard fires AFTER the write")
	}
}

func TestSetOperProfile_OutOfRange(t *testing.T) {
	p := profile("TrakRF", "1,2", map[int]float64{1: 30.0})
	f := &fakeOps{profiles: []Profile{p, p}}
	a := newAdapter(f)
	_, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{
		Antennas: []readerrpc.AntennaConfig{{Antenna: 1, Enabled: true, PowerDBm: 99.0}},
	}, false)
	if err == nil {
		t.Fatal("expected out-of-range error")
	}
	if f.getCalls != 0 || f.setProfileID != "" {
		t.Error("must validate before any reader call/write")
	}
}

func TestSetOperProfile_Busy(t *testing.T) {
	f := &fakeOps{holderIP: "192.168.50.203", profiles: []Profile{profile("TrakRF", "1,2", nil)}}
	a := newAdapter(f)
	_, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{
		Antennas: []readerrpc.AntennaConfig{{Antenna: 1, Enabled: true, PowerDBm: 25.0}},
	}, false)
	var be *readerrpc.BusyError
	if !errors.As(err, &be) {
		t.Fatalf("want *readerrpc.BusyError, got %v", err)
	}
	if f.setProfileID != "" {
		t.Error("must not write while busy")
	}
}

func TestSetOperProfile_DwellAppliedToAllPorts(t *testing.T) {
	// dwell change only (no antennas, no event): servlet RMW preserves enablement,
	// sets dwellTime{1..N} to the requested value via profileFields.
	p := profile("TrakRF", "1,2", map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0})
	f := &fakeOps{profiles: []Profile{p, p}}
	a := newAdapter(f)
	dwell := 400
	if _, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{DwellMs: &dwell}, false); err != nil {
		t.Fatalf("SetOperProfile(dwell): %v", err)
	}
	for port := 1; port <= 4; port++ {
		if got := f.setFields["dwellTime"+strconv.Itoa(port)]; got != "400" {
			t.Errorf("dwellTime%d = %q, want 400", port, got)
		}
	}
	// enablement unchanged (RMW preserves current antenna_port set).
	if !reflect.DeepEqual(f.setPorts, []int{1, 2}) {
		t.Errorf("enabled ports = %v, want [1 2]", f.setPorts)
	}
	// no event write for a dwell-only change.
	if len(f.modEvents) != 0 {
		t.Errorf("unexpected ModEvent calls: %v", f.modEvents)
	}
}

func TestSetOperProfile_EventKnobsOnly(t *testing.T) {
	// dedup/antDiff change only: no servlet write, modEvent carries the overrides,
	// event re-armed.
	f := &fakeOps{} // no profile read needed for an event-only change
	a := newAdapter(f)
	dedup := 750
	diff := false
	if _, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{
		DedupWindowMs: &dedup, AntennaDifferentiation: &diff,
	}, false); err != nil {
		t.Fatalf("SetOperProfile(event): %v", err)
	}
	if f.setProfileID != "" {
		t.Error("must NOT do a servlet profile write for an event-only change")
	}
	if len(f.modEvents) != 1 {
		t.Fatalf("want 1 ModEvent, got %d", len(f.modEvents))
	}
	if got := f.modEvents[0].Get("duplicateEliminationWindow"); got != "750" {
		t.Errorf("dedup = %q, want 750", got)
	}
	if got := f.modEvents[0].Get("antennaDifferentiation"); got != "false" {
		t.Errorf("antDiff = %q, want false", got)
	}
	if !reflect.DeepEqual(f.enableSeq, []bool{false, true}) {
		t.Errorf("re-arm = %v, want [false true]", f.enableSeq)
	}
}

func TestSetOperProfile_NothingToSet(t *testing.T) {
	f := &fakeOps{}
	a := newAdapter(f)
	if _, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{}, false); err == nil {
		t.Fatal("expected error when no settable fields provided")
	}
}

func TestSetOperProfile_PhaseSequencingReleasesSessions(t *testing.T) {
	// The CS463 single-session lock requires: /API session released BEFORE the
	// servlet form login, and the web session released BEFORE the 2nd /API login.
	p := profile("TrakRF", "1", map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0})
	f := &fakeOps{profiles: []Profile{p, p}}
	a := newAdapter(f)

	if _, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{
		Antennas: []readerrpc.AntennaConfig{{Antenna: 1, Enabled: true, PowerDBm: 25.0}},
	}, false); err != nil {
		t.Fatalf("SetOperProfile: %v", err)
	}

	want := []string{
		"Login",            // Phase A: read via /API
		"GetActiveProfile", // Phase A
		"Logout",           // Phase A: release /API session BEFORE form login
		"LoginServlet",     // Phase B: web/cookie auth
		"SetProfilePower",  // Phase B: write
		"LogoutServlet",    // Phase B: release web session BEFORE 2nd /API login
		"Login",            // Phase C: verify + re-arm via /API
		"GetActiveProfile", // Phase C: verify re-read BEFORE re-arm (profile still active)
		"EnableEvent(false)",
		"EnableEvent(true)",
		"Logout", // Phase C: release /API session
	}
	if !reflect.DeepEqual(f.callOrder, want) {
		t.Errorf("call order mismatch\n got: %v\nwant: %v", f.callOrder, want)
	}
}

func TestSetOperProfile_LoginServletBeforeWrite(t *testing.T) {
	p := profile("TrakRF", "1", map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0})
	f := &fakeOps{profiles: []Profile{p, p}}
	a := newAdapter(f)

	if _, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{
		Antennas: []readerrpc.AntennaConfig{{Antenna: 1, Enabled: true, PowerDBm: 25.0}},
	}, false); err != nil {
		t.Fatalf("SetOperProfile: %v", err)
	}

	// LoginServlet must be called, and must precede the servlet write.
	li, si := -1, -1
	for i, m := range f.callOrder {
		if m == "LoginServlet" && li == -1 {
			li = i
		}
		if m == "SetProfilePower" && si == -1 {
			si = i
		}
	}
	if li == -1 {
		t.Fatalf("LoginServlet was never called (order=%v)", f.callOrder)
	}
	if si == -1 || li >= si {
		t.Fatalf("LoginServlet must run before SetProfilePower (order=%v)", f.callOrder)
	}
}

func TestSetOperProfile_LoginServletErrorAbortsWrite(t *testing.T) {
	p := profile("TrakRF", "1", map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0})
	f := &fakeOps{profiles: []Profile{p, p}, loginServletEr: errors.New("login html")}
	a := newAdapter(f)

	_, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{
		Antennas: []readerrpc.AntennaConfig{{Antenna: 1, Enabled: true, PowerDBm: 25.0}},
	}, false)
	if err == nil {
		t.Fatal("expected LoginServlet error to abort SetOperProfile")
	}
	if f.setProfileID != "" {
		t.Error("must NOT write after LoginServlet failure")
	}
	for _, m := range f.callOrder {
		if m == "SetProfilePower" {
			t.Fatal("SetProfilePower must not be called after LoginServlet error")
		}
	}
}

func TestSetOperProfile_PropagatesWriteError(t *testing.T) {
	p := profile("TrakRF", "1", map[int]float64{1: 30.0})
	f := &fakeOps{profiles: []Profile{p, p}, setErr: errors.New("servlet boom")}
	a := newAdapter(f)
	_, err := a.SetOperProfile(context.Background(), readerrpc.ReaderConfig{
		Antennas: []readerrpc.AntennaConfig{{Antenna: 1, Enabled: true, PowerDBm: 25.0}},
	}, false)
	if err == nil {
		t.Fatal("expected write error to propagate")
	}
}

func TestGetStatus_Reading(t *testing.T) {
	f := &fakeOps{profiles: []Profile{profile("TrakRF", "1,2", nil)}}
	a := newAdapter(f)
	st, err := a.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if !st.Online || !st.Reading || st.ActiveProfile != "TrakRF" {
		t.Errorf("status = %+v, want online+reading+TrakRF", st)
	}
}

func TestGetStatus_NotReading(t *testing.T) {
	f := &fakeOps{profiles: []Profile{profile("TrakRF", "", nil)}}
	a := newAdapter(f)
	st, err := a.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if st.Reading {
		t.Error("reading should be false when antenna_port empty")
	}
}

func TestParseAntennaPorts(t *testing.T) {
	cases := map[string][]int{
		"1,2,4":   {1, 2, 4},
		"1":       {1},
		"":        nil,
		" 1 , 3 ": {1, 3},
	}
	for in, want := range cases {
		got := parseAntennaPorts(in)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("parseAntennaPorts(%q) = %v, want %v", in, got, want)
		}
	}
}

// --- Gpo.Set --------------------------------------------------------------

func (f *fakeOps) gpoSnapshot() []gpoCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]gpoCall(nil), f.gpoCalls...)
}

// A plain on with no pulse must drive the port and leave it latched: no release
// is scheduled, so the output stays on until an explicit off.
func TestGpoSet_NoPulseLatchesOn(t *testing.T) {
	f := &fakeOps{}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4})

	if err := a.GpoSet(context.Background(), 1, true, 0); err != nil {
		t.Fatalf("GpoSet: %v", err)
	}
	time.Sleep(30 * time.Millisecond) // give any (incorrectly) scheduled release a chance to fire

	got := f.gpoSnapshot()
	want := []gpoCall{{port: 1, on: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %+v, want %+v (a zero pulse must not schedule a release)", got, want)
	}
}

// pulse_ms arms a reader-side one-shot: on now, off after the delay, with no
// second request. This is the Shelly toggle_after analog the geofence engine's
// auto_off_seconds maps onto.
func TestGpoSet_PulseReleasesAfterDelay(t *testing.T) {
	f := &fakeOps{}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4})

	if err := a.GpoSet(context.Background(), 2, true, 40); err != nil {
		t.Fatalf("GpoSet: %v", err)
	}

	// Returns immediately, before the pulse expires: the caller is acknowledged
	// as soon as the port is energised rather than waiting out the pulse.
	if got := f.gpoSnapshot(); len(got) != 1 || !got[0].on {
		t.Fatalf("calls immediately after GpoSet = %+v, want exactly one on-call", got)
	}

	time.Sleep(120 * time.Millisecond)
	got := f.gpoSnapshot()
	want := []gpoCall{{port: 2, on: true}, {port: 2, on: false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %+v, want %+v", got, want)
	}
}

// An off command never schedules a release, whatever pulse_ms says.
func TestGpoSet_OffIgnoresPulse(t *testing.T) {
	f := &fakeOps{}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4})

	if err := a.GpoSet(context.Background(), 3, false, 40); err != nil {
		t.Fatalf("GpoSet: %v", err)
	}
	time.Sleep(120 * time.Millisecond)

	got := f.gpoSnapshot()
	want := []gpoCall{{port: 3, on: false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %+v, want %+v", got, want)
	}
}

// A transport failure surfaces to the caller and schedules nothing.
func TestGpoSet_TransportErrorPropagates(t *testing.T) {
	f := &fakeOps{gpoErr: errors.New("reader unreachable")}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4})

	if err := a.GpoSet(context.Background(), 1, true, 40); err == nil {
		t.Fatal("expected error from transport")
	}
	time.Sleep(120 * time.Millisecond)

	if got := f.gpoSnapshot(); len(got) != 1 {
		t.Errorf("calls = %+v, want exactly one (no release after a failed on)", got)
	}
}

// TRA-1028 whole-branch-review finding: each pulsed Gpo.Set used to spawn an
// INDEPENDENT release goroutine with no per-port state. If a second tag exited
// (a second GpoSet on the same port) before the first pulse elapsed, the FIRST
// timer still fired at its ORIGINAL deadline and cut the second, longer alarm
// short. A per-port timer must be superseded — Stop()'d and replaced — not
// raced. This mirrors Shelly's toggle_after REPLACE semantics.
//
// The bug is proven by checking the port state at a time PAST the first
// pulse's original deadline but BEFORE the second pulse's deadline: if the
// stale first timer fired, the port would already show an (incorrect) off
// call at that checkpoint.
func TestGpoSet_LongerPulseSupersedesPendingRelease(t *testing.T) {
	f := &fakeOps{}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4})

	if err := a.GpoSet(context.Background(), 5, true, 30); err != nil {
		t.Fatalf("GpoSet #1: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // well before pulse #1's 30ms deadline

	// A second, longer pulse arrives on the SAME port before #1 elapses — the
	// two-tags-in-one-window scenario from the finding.
	if err := a.GpoSet(context.Background(), 5, true, 150); err != nil {
		t.Fatalf("GpoSet #2: %v", err)
	}

	// Checkpoint: past #1's original deadline (~30ms from its call, i.e. ~40ms
	// from test start) but well before #2's (~10+150=160ms from test start).
	time.Sleep(60 * time.Millisecond) // ~70ms since test start
	got := f.gpoSnapshot()
	want := []gpoCall{{port: 5, on: true}, {port: 5, on: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls at the ~70ms checkpoint = %+v, want %+v (pulse #1's stale timer must NOT have released the port)", got, want)
	}

	// Past #2's deadline: exactly one release, timed off the SECOND pulse.
	time.Sleep(150 * time.Millisecond) // ~220ms since test start
	got = f.gpoSnapshot()
	want = []gpoCall{{port: 5, on: true}, {port: 5, on: true}, {port: 5, on: false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("final calls = %+v, want %+v (exactly one release, driven by pulse #2's timer)", got, want)
	}
}

// The explicit re-fire/latch case from the finding: an on:true,pulse_ms:0
// (latch forever) call arriving while a pulse is pending must not be killed
// by the stale pending timer either — the latch must win permanently.
func TestGpoSet_LatchSupersedesPendingRelease(t *testing.T) {
	f := &fakeOps{}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4})

	if err := a.GpoSet(context.Background(), 6, true, 30); err != nil {
		t.Fatalf("GpoSet #1 (pulse): %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	if err := a.GpoSet(context.Background(), 6, true, 0); err != nil {
		t.Fatalf("GpoSet #2 (latch): %v", err)
	}

	// Wait well past pulse #1's original deadline; the latch must hold.
	time.Sleep(80 * time.Millisecond)
	got := f.gpoSnapshot()
	want := []gpoCall{{port: 6, on: true}, {port: 6, on: true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %+v, want %+v (latch must survive the superseded pulse's deadline)", got, want)
	}
}

// An explicit off arriving while a pulse is pending must cancel the pending
// release outright — there must be exactly one off call (the explicit one),
// never a second one from the stale timer firing later.
func TestGpoSet_OffCancelsPendingRelease(t *testing.T) {
	f := &fakeOps{}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4})

	if err := a.GpoSet(context.Background(), 7, true, 30); err != nil {
		t.Fatalf("GpoSet #1 (pulse): %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	if err := a.GpoSet(context.Background(), 7, false, 0); err != nil {
		t.Fatalf("GpoSet #2 (explicit off): %v", err)
	}

	// Wait well past pulse #1's original deadline: no second off must appear.
	time.Sleep(80 * time.Millisecond)
	got := f.gpoSnapshot()
	want := []gpoCall{{port: 7, on: true}, {port: 7, on: false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %+v, want %+v (the stale pulse timer must have been cancelled, not just superseded in effect)", got, want)
	}
}
