package cs463

import (
	"context"
	"net/url"
	"testing"
)

// fakeReco implements reconcileOps (entityOps + session lifecycle + event re-arm)
// with in-memory rows and write counters.
type fakeReco struct {
	rows     map[string]map[string]EntityRow // entity kind -> id -> row
	servers  map[string]EntityRow
	profiles map[string]bool
	adds     map[string]int
	mods     map[string]int

	// session lifecycle / re-arm tracking (for Adapter.Reconcile tests)
	holderIP  string
	enableSeq []bool // EnableEvent enable flags in call order

	// profile-create tracking (for ensureProfile tests)
	createProfileCalls int
	servletPorts       []int // enabledPorts passed to SetProfilePower
}

func newFakeReco() *fakeReco {
	return &fakeReco{
		rows:     map[string]map[string]EntityRow{"dataformat": {}, "trigger": {}, "action": {}, "event": {}},
		servers:  map[string]EntityRow{},
		profiles: map[string]bool{},
		adds:     map[string]int{},
		mods:     map[string]int{},
	}
}

// newMatchingFake seeds every entity with a golden-matching row plus the required
// pre-created server + profile.
func newMatchingFake(antennaCount int) *fakeReco {
	f := newFakeReco()
	f.servers[NameMQTTServer] = EntityRow{"server_id": NameMQTTServer, "type": "MQTT"}
	f.profiles[NameProfile] = true
	f.rows["dataformat"][NameDataFormat] = EntityRow{
		"format": "JSON",
		"field1": "SequenceNumber", "label1": "sequenceNumber",
		"field2": "NumberOfTags", "label2": "numberOfTags",
		"field3": "TagDataList", "label3": "tags",
		"tagDataField1": "EPC", "tagDataLabel1": "epc",
		"tagDataField2": "TimeStampOfRead", "tagDataLabel2": "timeStampOfRead",
		"tagDataField3": "AntennaPort_Number", "tagDataLabel3": "antennaPort",
		"tagDataField4": "RSSI_Number", "tagDataLabel4": "rssi",
	}
	f.rows["trigger"][NameTrigger] = EntityRow{
		"mode": triggerModeRSSIGate, "logic": "-80", "capture_point": "12",
	}
	f.rows["action"][NameAction] = EntityRow{
		"server_id": NameMQTTServer, "data_format_id": NameDataFormat,
		"transport": "MQTT", "action_mode": actionModeLowLat,
	}
	f.rows["event"][NameEvent] = EntityRow{
		"operProfile_id": NameProfile, "triggering_logic": NameTrigger,
		"resultant_action": NameAction, "exclusivity": "Non-exclusive",
		"duplicateEliminationWindow": "500", "antennaDifferentiation": "true", "enable": "true",
	}
	return f
}

func (f *fakeReco) totalWrites() int {
	n := 0
	for _, v := range f.adds {
		n += v
	}
	for _, v := range f.mods {
		n += v
	}
	return n
}

func (f *fakeReco) Login(ctx context.Context) (string, string, error) {
	if f.holderIP != "" {
		return "", f.holderIP, nil
	}
	return "sess1", "", nil
}
func (f *fakeReco) Logout(ctx context.Context, s string) error { return nil }
func (f *fakeReco) EnableEvent(ctx context.Context, s, eventID string, enable bool) error {
	f.enableSeq = append(f.enableSeq, enable)
	return nil
}

// --- readerOps methods (so *fakeReco can also be the Adapter's a.ops for ensureProfile) ---
func (f *fakeReco) ForceLogout(ctx context.Context) error { return nil }
func (f *fakeReco) GetActiveProfile(ctx context.Context, s string) (Profile, error) {
	return Profile{ID: NameProfile}, nil
}
func (f *fakeReco) LoginServlet(ctx context.Context) error  { return nil }
func (f *fakeReco) LogoutServlet(ctx context.Context) error { return nil }
func (f *fakeReco) SetProfilePower(ctx context.Context, profileID string, antennaCount int, enabledPorts []int, powers map[int]float64, profileFields map[string]string) error {
	f.servletPorts = enabledPorts
	return nil
}
func (f *fakeReco) CreateProfile(ctx context.Context, s, profileID string, txPowerDBm float64) error {
	f.createProfileCalls++
	f.profiles[profileID] = true // now it exists
	return nil
}

func (f *fakeReco) ListServer(ctx context.Context, s string) (map[string]EntityRow, error) {
	return f.servers, nil
}
func (f *fakeReco) ListProfileIDs(ctx context.Context, s string) (map[string]bool, error) {
	return f.profiles, nil
}
func (f *fakeReco) ListDataFormat(ctx context.Context, s string) (map[string]EntityRow, error) {
	return f.rows["dataformat"], nil
}
func (f *fakeReco) ListTriggeringLogic(ctx context.Context, s string) (map[string]EntityRow, error) {
	return f.rows["trigger"], nil
}
func (f *fakeReco) ListResultantAction(ctx context.Context, s string) (map[string]EntityRow, error) {
	return f.rows["action"], nil
}
func (f *fakeReco) ListEvent(ctx context.Context, s string) (map[string]EntityRow, error) {
	return f.rows["event"], nil
}
func (f *fakeReco) AddDataFormat(ctx context.Context, s string, p url.Values) error {
	f.adds["dataformat"]++
	return nil
}
func (f *fakeReco) ModDataFormat(ctx context.Context, s string, p url.Values) error {
	f.mods["dataformat"]++
	return nil
}
func (f *fakeReco) AddTriggeringLogic(ctx context.Context, s string, p url.Values) error {
	f.adds["trigger"]++
	return nil
}
func (f *fakeReco) ModTriggeringLogic(ctx context.Context, s string, p url.Values) error {
	f.mods["trigger"]++
	return nil
}
func (f *fakeReco) AddResultantAction(ctx context.Context, s string, p url.Values) error {
	f.adds["action"]++
	return nil
}
func (f *fakeReco) ModResultantAction(ctx context.Context, s string, p url.Values) error {
	f.mods["action"]++
	return nil
}
func (f *fakeReco) AddEvent(ctx context.Context, s string, p url.Values) error {
	f.adds["event"]++
	return nil
}
func (f *fakeReco) ModEvent(ctx context.Context, s string, p url.Values) error {
	f.mods["event"]++
	return nil
}

func TestReconcileCreatesWhenAbsent(t *testing.T) {
	f := newFakeReco()
	changed, err := reconcileGolden(context.Background(), "sid", f, 2)
	if err != nil || !changed {
		t.Fatalf("want changed=true err=nil, got changed=%v err=%v", changed, err)
	}
	for _, k := range []string{"dataformat", "trigger", "action", "event"} {
		if f.adds[k] != 1 {
			t.Errorf("expected one add for %s, got %d", k, f.adds[k])
		}
	}
	if f.totalWrites() != 4 {
		t.Errorf("expected exactly 4 writes (one add each), got %d", f.totalWrites())
	}
}

func TestReconcileNoOpWhenMatches(t *testing.T) {
	f := newMatchingFake(2)
	changed, err := reconcileGolden(context.Background(), "sid", f, 2)
	if err != nil || changed {
		t.Fatalf("want changed=false err=nil, got changed=%v err=%v", changed, err)
	}
	if f.totalWrites() != 0 {
		t.Fatalf("converged reader must produce zero writes, got %d (adds=%v mods=%v)", f.totalWrites(), f.adds, f.mods)
	}
}

func TestReconcileModsOnDrift(t *testing.T) {
	// Structural drift (wrong operProfile_id) must trigger a mod.
	f := newMatchingFake(2)
	f.rows["event"][NameEvent]["operProfile_id"] = "SomeOtherProfile" // structural drift
	changed, err := reconcileGolden(context.Background(), "sid", f, 2)
	if err != nil || !changed {
		t.Fatalf("want changed=true, got changed=%v err=%v", changed, err)
	}
	if f.mods["event"] != 1 || f.adds["event"] != 0 {
		t.Fatalf("expected one event mod (no add), got mods=%v adds=%v", f.mods, f.adds)
	}
	if f.totalWrites() != 1 {
		t.Fatalf("only the drifted event should be written, got %d total", f.totalWrites())
	}
}

func TestReconcileNoOpWhenOnlyDedupOrAntDiffDiffers(t *testing.T) {
	// dedup/antDiff are customer-editable (TRA-1003): reconcile must NOT
	// revert them on restart (they are not structural drift).
	f := newMatchingFake(2)
	f.rows["event"][NameEvent]["duplicateEliminationWindow"] = "5000"
	f.rows["event"][NameEvent]["antennaDifferentiation"] = "false"
	changed, err := reconcileGolden(context.Background(), "sid", f, 2)
	if err != nil {
		t.Fatalf("reconcileGolden: %v", err)
	}
	if changed {
		t.Error("dedup/antDiff change must NOT trigger a reconcile write (customer-editable knobs)")
	}
	if f.mods["event"] != 0 {
		t.Errorf("expected zero event mods, got %d", f.mods["event"])
	}
}

func TestVerifyServer(t *testing.T) {
	f := newMatchingFake(2)
	if err := verifyServer(context.Background(), "sid", f); err != nil {
		t.Fatalf("present server must verify, got %v", err)
	}
	noServer := newMatchingFake(2)
	delete(noServer.servers, NameMQTTServer)
	if err := verifyServer(context.Background(), "sid", noServer); err == nil {
		t.Error("missing CloudServer must fail verify")
	}
}

func TestEnsureProfileCreatesWhenAbsent(t *testing.T) {
	f := newMatchingFake(2)
	delete(f.profiles, NameProfile) // profile absent -> daemon creates it
	a := &Adapter{cfg: AdapterConfig{AntennaCount: 4}, rec: f, ops: f}
	if err := a.ensureProfile(context.Background()); err != nil {
		t.Fatalf("ensureProfile: %v", err)
	}
	if f.createProfileCalls != 1 {
		t.Fatalf("absent profile must be created once, got %d", f.createProfileCalls)
	}
	if len(f.servletPorts) != 1 || f.servletPorts[0] != 1 {
		t.Fatalf("created profile must enable antenna 1 via servlet, got ports %v", f.servletPorts)
	}
}

func TestEnsureProfileHandsOffWhenPresent(t *testing.T) {
	f := newMatchingFake(2) // profile present
	a := &Adapter{cfg: AdapterConfig{AntennaCount: 4}, rec: f, ops: f}
	if err := a.ensureProfile(context.Background()); err != nil {
		t.Fatalf("ensureProfile: %v", err)
	}
	if f.createProfileCalls != 0 || f.servletPorts != nil {
		t.Fatalf("existing profile must be left untouched, got creates=%d servletPorts=%v", f.createProfileCalls, f.servletPorts)
	}
}

func TestAdapterReconcileNoOpStillRearms(t *testing.T) {
	// Even a converged (no-drift) reconcile must re-arm: the CS463 does not reliably
	// auto-start inventory, so the daemon arms defensively on every startup.
	f := newMatchingFake(2) // converged: no drift
	a := &Adapter{cfg: AdapterConfig{AntennaCount: 2, EventID: "ignored"}, rec: f, ops: f}
	if err := a.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if f.totalWrites() != 0 {
		t.Fatalf("no-op reconcile must not write entities, got %d", f.totalWrites())
	}
	if len(f.enableSeq) != 2 || f.enableSeq[0] != false || f.enableSeq[1] != true {
		t.Fatalf("startup reconcile must always re-arm (disable, enable); got %v", f.enableSeq)
	}
}

func TestAdapterReconcileRearmsWhenChanged(t *testing.T) {
	f := newMatchingFake(2)
	f.rows["event"][NameEvent]["operProfile_id"] = "SomeOtherProfile" // structural drift -> mod
	a := &Adapter{cfg: AdapterConfig{AntennaCount: 2, EventID: "ignored"}, rec: f, ops: f}
	if err := a.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	// re-arm = disable then enable, on the golden event.
	if len(f.enableSeq) != 2 || f.enableSeq[0] != false || f.enableSeq[1] != true {
		t.Fatalf("changed reconcile must re-arm (disable, enable); got %v", f.enableSeq)
	}
}

func TestAdapterReconcileFailsOnMissingServer(t *testing.T) {
	f := newMatchingFake(2)
	delete(f.servers, NameMQTTServer)
	a := &Adapter{cfg: AdapterConfig{AntennaCount: 2}, rec: f, ops: f}
	if err := a.Reconcile(context.Background()); err == nil {
		t.Fatal("reconcile must fail when the hand-crafted CloudServer is absent")
	}
	if f.totalWrites() != 0 {
		t.Fatalf("must not write entities when verify fails, got %d", f.totalWrites())
	}
}

func TestAdapterReconcileBusyReader(t *testing.T) {
	f := newMatchingFake(2)
	f.holderIP = "192.168.50.9"
	a := &Adapter{cfg: AdapterConfig{AntennaCount: 2}, rec: f, ops: f}
	if err := a.Reconcile(context.Background()); err == nil {
		t.Fatal("reconcile must fail when the reader is busy (single-session lock)")
	}
}

func TestAdapterReconcileUnsupportedOps(t *testing.T) {
	// an adapter whose ops do not satisfy reconcileOps cannot reconcile.
	a := &Adapter{cfg: AdapterConfig{AntennaCount: 2}}
	if err := a.Reconcile(context.Background()); err == nil {
		t.Fatal("reconcile must error when rec is nil")
	}
}
