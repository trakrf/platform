package cs463

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/trakrf/platform/backend/internal/readerrpc"
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

func (f *fakeOps) EnableEvent(ctx context.Context, session, eventID string, enable bool) error {
	if enable {
		f.callOrder = append(f.callOrder, "EnableEvent(true)")
	} else {
		f.callOrder = append(f.callOrder, "EnableEvent(false)")
	}
	f.enableSeq = append(f.enableSeq, enable)
	return f.enableEr
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
		readerrpc.MethodGetConfig,
		readerrpc.MethodSetConfig,
		readerrpc.MethodGetStatus,
	}
	if !reflect.DeepEqual(caps.Supports, wantSupports) {
		t.Errorf("supports = %v, want %v", caps.Supports, wantSupports)
	}
	wantUnsupported := []string{
		readerrpc.MethodScanStart,
		readerrpc.MethodScanStop,
		readerrpc.MethodGpoSet,
		readerrpc.MethodReboot,
	}
	if !reflect.DeepEqual(caps.Unsupported, wantUnsupported) {
		t.Errorf("unsupported = %v, want %v", caps.Unsupported, wantUnsupported)
	}
}

func TestGetConfig_MapsPowersSorted(t *testing.T) {
	f := &fakeOps{profiles: []Profile{
		profile("TrakRF", "1,2", map[int]float64{3: 0.0, 1: 30.0, 2: 22.5, 4: 0.0}),
	}}
	a := newAdapter(f)
	cfg, err := a.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	want := []readerrpc.AntennaPower{
		{Antenna: 1, Power: 30.0},
		{Antenna: 2, Power: 22.5},
		{Antenna: 3, Power: 0.0},
		{Antenna: 4, Power: 0.0},
	}
	if !reflect.DeepEqual(cfg.TxPowerDBm, want) {
		t.Errorf("tx power = %+v, want %+v", cfg.TxPowerDBm, want)
	}
	if !f.loggedOut {
		t.Error("expected logout")
	}
}

func TestSetConfig_HappyPath(t *testing.T) {
	// active profile: ports 1,2 enabled, powers 1=30,2=22.5,3=0,4=0
	p := profile("TrakRF", "1,2", map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0})
	f := &fakeOps{profiles: []Profile{p, p}} // 2nd read unchanged -> verify passes
	a := newAdapter(f)

	res, err := a.SetConfig(context.Background(), readerrpc.ReaderConfig{
		TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 25.0}},
	})
	if err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if res.Applied != readerrpc.AppliedPendingReload {
		t.Errorf("applied = %q, want %q", res.Applied, readerrpc.AppliedPendingReload)
	}
	if res.EffectiveAt != "next_inventory_cycle" {
		t.Errorf("effective_at = %q", res.EffectiveAt)
	}
	if f.setProfileID != "TrakRF" {
		t.Errorf("set profile id = %q", f.setProfileID)
	}
	if !reflect.DeepEqual(f.setPorts, []int{1, 2}) {
		t.Errorf("enabled ports = %v, want [1 2]", f.setPorts)
	}
	// merged: port 1 overridden to 25, rest from current
	wantPowers := map[int]float64{1: 25.0, 2: 22.5, 3: 0.0, 4: 0.0}
	if !reflect.DeepEqual(f.setPowers, wantPowers) {
		t.Errorf("powers = %v, want %v", f.setPowers, wantPowers)
	}
	// re-arm: disable THEN enable
	if !reflect.DeepEqual(f.enableSeq, []bool{false, true}) {
		t.Errorf("enable sequence = %v, want [false true]", f.enableSeq)
	}
	if !f.loggedOut {
		t.Error("expected logout")
	}
}

func TestSetConfig_LoginServletBeforeWrite(t *testing.T) {
	p := profile("TrakRF", "1,2", map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0})
	f := &fakeOps{profiles: []Profile{p, p}}
	a := newAdapter(f)

	if _, err := a.SetConfig(context.Background(), readerrpc.ReaderConfig{
		TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 25.0}},
	}); err != nil {
		t.Fatalf("SetConfig: %v", err)
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

func TestSetConfig_PhaseSequencingReleasesSessions(t *testing.T) {
	// The CS463 single-session lock requires: /API session released BEFORE the
	// servlet form login, and the web session released BEFORE the 2nd /API login.
	p := profile("TrakRF", "1,2", map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0})
	f := &fakeOps{profiles: []Profile{p, p}}
	a := newAdapter(f)

	if _, err := a.SetConfig(context.Background(), readerrpc.ReaderConfig{
		TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 25.0}},
	}); err != nil {
		t.Fatalf("SetConfig: %v", err)
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

func TestSetConfig_LoginServletErrorAbortsWrite(t *testing.T) {
	p := profile("TrakRF", "1,2", map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0})
	f := &fakeOps{profiles: []Profile{p, p}, loginServletEr: errors.New("login html")}
	a := newAdapter(f)

	_, err := a.SetConfig(context.Background(), readerrpc.ReaderConfig{
		TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 25.0}},
	})
	if err == nil {
		t.Fatal("expected LoginServlet error to abort SetConfig")
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

func TestSetConfig_OutOfRange(t *testing.T) {
	p := profile("TrakRF", "1,2", map[int]float64{1: 30.0})
	f := &fakeOps{profiles: []Profile{p, p}}
	a := newAdapter(f)

	_, err := a.SetConfig(context.Background(), readerrpc.ReaderConfig{
		TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 99.0}},
	})
	if err == nil {
		t.Fatal("expected out-of-range error")
	}
	if f.getCalls != 0 || f.setProfileID != "" {
		t.Error("must validate before any reader call/write")
	}
}

func TestSetConfig_Busy(t *testing.T) {
	f := &fakeOps{holderIP: "192.168.50.203", profiles: []Profile{profile("TrakRF", "1,2", nil)}}
	a := newAdapter(f)
	_, err := a.SetConfig(context.Background(), readerrpc.ReaderConfig{
		TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 25.0}},
	})
	if err == nil {
		t.Fatal("expected busy error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "use") {
		t.Errorf("busy error should mention reader in use: %v", err)
	}
	if f.setProfileID != "" {
		t.Error("must not write while busy")
	}
}

func TestSetConfig_AntennaWipeGuard(t *testing.T) {
	before := profile("TrakRF", "1,2", map[int]float64{1: 30.0, 2: 22.5, 3: 0.0, 4: 0.0})
	after := profile("TrakRF", "", map[int]float64{1: 25.0}) // wiped antenna_port
	f := &fakeOps{profiles: []Profile{before, after}}
	a := newAdapter(f)

	_, err := a.SetConfig(context.Background(), readerrpc.ReaderConfig{
		TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 25.0}},
	})
	if err == nil {
		t.Fatal("expected antenna-wipe guard error")
	}
	if f.setProfileID == "" {
		t.Error("guard fires AFTER the write")
	}
}

func TestSetConfig_PropagatesWriteError(t *testing.T) {
	p := profile("TrakRF", "1,2", map[int]float64{1: 30.0})
	f := &fakeOps{profiles: []Profile{p, p}, setErr: errors.New("servlet boom")}
	a := newAdapter(f)
	_, err := a.SetConfig(context.Background(), readerrpc.ReaderConfig{
		TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 25.0}},
	})
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
