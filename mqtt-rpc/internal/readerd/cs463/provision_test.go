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
	f := newMatchingFake(2)
	f.rows["event"][NameEvent]["duplicateEliminationWindow"] = "5000" // drift
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

func TestVerifyServerAndProfile(t *testing.T) {
	f := newMatchingFake(2)
	if err := verifyServerAndProfile(context.Background(), "sid", f); err != nil {
		t.Fatalf("present server+profile must verify, got %v", err)
	}
	noServer := newMatchingFake(2)
	delete(noServer.servers, NameMQTTServer)
	if err := verifyServerAndProfile(context.Background(), "sid", noServer); err == nil {
		t.Error("missing CloudServer must fail verify")
	}
	noProfile := newMatchingFake(2)
	delete(noProfile.profiles, NameProfile)
	if err := verifyServerAndProfile(context.Background(), "sid", noProfile); err == nil {
		t.Error("missing profile must fail verify")
	}
}

func TestAdapterReconcileNoOpStillRearms(t *testing.T) {
	// Even a converged (no-drift) reconcile must re-arm: the CS463 does not auto-start
	// inventory after a reboot/restart, and the daemon starts after every reader boot.
	f := newMatchingFake(2) // converged: no drift
	a := &Adapter{cfg: AdapterConfig{AntennaCount: 2, EventID: "ignored"}, rec: f}
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
	f.rows["event"][NameEvent]["duplicateEliminationWindow"] = "5000" // drift -> mod
	a := &Adapter{cfg: AdapterConfig{AntennaCount: 2, EventID: "ignored"}, rec: f}
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
	a := &Adapter{cfg: AdapterConfig{AntennaCount: 2}, rec: f}
	if err := a.Reconcile(context.Background()); err == nil {
		t.Fatal("reconcile must fail when the pre-created CloudServer is absent")
	}
	if f.totalWrites() != 0 {
		t.Fatalf("must not write entities when verify fails, got %d", f.totalWrites())
	}
}

func TestAdapterReconcileBusyReader(t *testing.T) {
	f := newMatchingFake(2)
	f.holderIP = "192.168.50.9"
	a := &Adapter{cfg: AdapterConfig{AntennaCount: 2}, rec: f}
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
