package readerd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

const (
	H = readerrpc.APIHealthy
	D = readerrpc.APIDegraded
	U = readerrpc.APIUnreachable
)

// rig drives a Monitor tick by tick on a fake clock, one probe result per tick.
type rig struct {
	t         *testing.T
	m         *Monitor
	now       time.Time
	probe     string
	reconcile []error // results to return, in order; nil entries succeed
	events    []string
	published []readerrpc.Health
	store     RebootStore
}

type rigHost struct{ r *rig }

func (h rigHost) RestartReaderApp(context.Context) error {
	h.r.events = append(h.r.events, "restart")
	return nil
}
func (h rigHost) Reboot() error {
	h.r.events = append(h.r.events, "reboot")
	return nil
}

func newRig(t *testing.T, p HealthPolicy, reconcile bool) *rig {
	t.Helper()
	r := &rig{
		t:     t,
		now:   time.Date(2026, 11, 8, 17, 0, 0, 0, time.UTC),
		store: RebootStore{Path: filepath.Join(t.TempDir(), "auto-reboots.json")},
	}
	log := zerolog.Nop()
	deps := MonitorDeps{
		Probe: func(context.Context) (string, string) {
			if r.probe == H {
				return H, ""
			}
			return r.probe, "probe said " + r.probe
		},
		Host:  rigHost{r},
		Store: r.store,
		Publish: func(h readerrpc.Health) {
			r.published = append(r.published, h)
		},
		GoingDown: func(reason string) { r.events = append(r.events, "going-down:"+reason) },
		Log:       &log,
		Now:       func() time.Time { return r.now },
	}
	if reconcile {
		deps.Reconcile = func(context.Context) error {
			r.events = append(r.events, "reconcile")
			if len(r.reconcile) == 0 {
				return nil
			}
			err := r.reconcile[0]
			r.reconcile = r.reconcile[1:]
			return err
		}
	}
	r.m = NewMonitor(p, deps)
	return r
}

// run ticks once per policy interval with the given probe result for d.
func (r *rig) run(state string, d time.Duration) {
	r.t.Helper()
	r.probe = state
	for end := r.now.Add(d); r.now.Before(end); r.now = r.now.Add(r.m.policy.Interval) {
		r.m.Tick(context.Background())
	}
}

func (r *rig) count(event string) int {
	n := 0
	for _, e := range r.events {
		if e == event {
			n++
		}
	}
	return n
}

func (r *rig) last() readerrpc.Health {
	if len(r.published) == 0 {
		r.t.Fatal("nothing published")
	}
	return r.published[len(r.published)-1]
}

func TestMonitor_HealthUnknownBeforeFirstTick(t *testing.T) {
	r := newRig(t, DefaultHealthPolicy(), true)
	h := r.m.Health()
	if h.API != readerrpc.APIUnknown || h.GoldenConfig != readerrpc.GoldenPending {
		t.Fatalf("initial health = %+v", h)
	}
	if off := newRig(t, DefaultHealthPolicy(), false).m.Health(); off.GoldenConfig != readerrpc.GoldenOff {
		t.Fatalf("reconcile off: golden_config = %q, want off", off.GoldenConfig)
	}
}

// The boot race (TRA-1041, 2026-11-08 02:02): the daemon starts, nginx answers 502
// for about two minutes while GlassFish deploys, then /API answers. That is not
// a wedge, and the reconcile that used to fail once and never retry must now run.
func TestMonitor_BootRaceReconcilesWithoutRecovery(t *testing.T) {
	r := newRig(t, DefaultHealthPolicy(), true)
	r.run(U, 3*time.Minute)
	r.run(H, 2*time.Minute)

	if r.count("reconcile") != 1 {
		t.Fatalf("reconciles = %d, want 1 (events %q)", r.count("reconcile"), r.events)
	}
	if r.count("restart")+r.count("reboot") != 0 {
		t.Fatalf("a normal boot took recovery action: %q", r.events)
	}
	if h := r.last(); h.API != H || h.Wedged || h.GoldenConfig != readerrpc.GoldenApplied {
		t.Fatalf("health after boot = %+v", h)
	}
}

// A reconcile that fails while the API is healthy (the session slot is busy,
// say) is retried on the retry interval, not on every probe.
func TestMonitor_ReconcileFailureRetriedOnInterval(t *testing.T) {
	r := newRig(t, DefaultHealthPolicy(), true)
	r.reconcile = []error{errors.New("reader busy"), nil}
	r.run(H, 4*time.Minute)
	if r.count("reconcile") != 1 {
		t.Fatalf("reconciles in 4 min = %d, want 1", r.count("reconcile"))
	}
	if r.last().GoldenConfig != readerrpc.GoldenPending {
		t.Fatalf("golden_config after a failed reconcile = %q, want pending", r.last().GoldenConfig)
	}
	r.run(H, 2*time.Minute)
	if r.count("reconcile") != 2 || r.last().GoldenConfig != readerrpc.GoldenApplied {
		t.Fatalf("reconciles = %d, golden = %q; want 2, applied", r.count("reconcile"), r.last().GoldenConfig)
	}
}

// An app restart stops inventory, so recovering from an outage re-runs the
// reconcile, which re-arms the golden event.
func TestMonitor_ReconcileAgainAfterRecovery(t *testing.T) {
	r := newRig(t, DefaultHealthPolicy(), true)
	r.run(H, time.Minute)
	r.run(D, 2*time.Minute)
	r.run(H, time.Minute)
	if r.count("reconcile") != 2 {
		t.Fatalf("reconciles = %d, want 2 (events %q)", r.count("reconcile"), r.events)
	}
}

// One failed probe is not an outage worth an inventory re-arm.
func TestMonitor_SingleBadProbeDoesNotReconcile(t *testing.T) {
	r := newRig(t, DefaultHealthPolicy(), true)
	r.run(H, time.Minute)
	r.run(U, r.m.policy.Interval)
	r.run(H, time.Minute)
	if r.count("reconcile") != 1 {
		t.Fatalf("reconciles = %d, want 1", r.count("reconcile"))
	}
}

func TestMonitor_WedgeLadder(t *testing.T) {
	p := DefaultHealthPolicy()
	r := newRig(t, p, true)
	r.run(H, time.Minute)

	r.run(D, p.WedgeAfter) // the ticks up to, not including, WedgeAfter
	if r.last().Wedged || len(r.events) != 1 {
		t.Fatalf("acted before the grace window: %q, %+v", r.events, r.last())
	}

	r.run(D, p.Interval)
	if h := r.last(); !h.Wedged || h.Recovery != readerrpc.RecoveryAppRestart || r.count("restart") != 1 {
		t.Fatalf("after the window: events %q, health %+v; want one app restart", r.events, h)
	}

	// The restart did not clear it: a further window brings the reboot.
	r.run(D, p.WedgeAfter)
	if r.count("restart") != 1 || r.count("reboot") != 1 {
		t.Fatalf("events %q, want one restart then one reboot", r.events)
	}
	tail := strings.Join(r.events[len(r.events)-2:], ",")
	if tail != "going-down:auto-recovery,reboot" {
		t.Fatalf("reboot sequence = %q, want going-down before reboot", tail)
	}
	if n, _ := r.store.Count(); n != 1 {
		t.Fatalf("recorded reboots = %d, want 1 (recorded before rebooting)", n)
	}
	if h := r.last(); h.Recovery != readerrpc.RecoveryReboot || h.AutoReboots != 1 {
		t.Fatalf("health at reboot = %+v", h)
	}
}

func TestMonitor_RecoveryAfterAppRestartClearsWedge(t *testing.T) {
	p := DefaultHealthPolicy()
	r := newRig(t, p, true)
	r.run(D, p.WedgeAfter)
	r.run(H, time.Minute)
	if h := r.last(); h.Wedged || h.Recovery != "" || h.GoldenConfig != readerrpc.GoldenApplied {
		t.Fatalf("health after recovery = %+v", h)
	}
	if r.count("reboot") != 0 {
		t.Fatalf("rebooted although the restart cleared it: %q", r.events)
	}
}

func TestMonitor_CapReachedStaysUpAndSaysSo(t *testing.T) {
	p := DefaultHealthPolicy()
	r := newRig(t, p, true)
	for i := 0; i < p.MaxAutoReboots; i++ {
		if err := r.store.Record(r.now); err != nil {
			t.Fatal(err)
		}
	}
	r.m = NewMonitor(p, r.m.deps)
	r.run(D, 3*p.WedgeAfter)
	if r.count("reboot") != 0 {
		t.Fatalf("rebooted past the cap: %q", r.events)
	}
	if h := r.last(); !h.Wedged || !h.AutoRebootsExhausted || h.AutoReboots != p.MaxAutoReboots {
		t.Fatalf("health past the cap = %+v", h)
	}
}

// A count it cannot read is a cap it cannot enforce: never reboot on it.
func TestMonitor_CorruptStoreNeverReboots(t *testing.T) {
	p := DefaultHealthPolicy()
	r := newRig(t, p, true)
	if err := os.WriteFile(r.store.Path, []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.run(D, 3*p.WedgeAfter)
	if r.count("reboot") != 0 {
		t.Fatalf("rebooted on an unreadable count: %q", r.events)
	}
	if !r.last().AutoRebootsExhausted {
		t.Fatalf("health = %+v, want auto_reboots_exhausted", r.last())
	}
}

func TestMonitor_AutoRecoverOffOnlyReports(t *testing.T) {
	p := DefaultHealthPolicy()
	p.AutoRecover = false
	r := newRig(t, p, true)
	r.run(D, 3*p.WedgeAfter)
	if r.count("restart")+r.count("reboot") != 0 {
		t.Fatalf("acted with auto-recovery off: %q", r.events)
	}
	if !r.last().Wedged {
		t.Fatalf("health = %+v, want wedged", r.last())
	}
}

func TestMonitor_SustainedHealthResetsTheCount(t *testing.T) {
	p := DefaultHealthPolicy()
	r := newRig(t, p, true)
	if err := r.store.Record(r.now); err != nil {
		t.Fatal(err)
	}
	r.m = NewMonitor(p, r.m.deps) // a fresh process reads the persisted count
	if r.m.Health().AutoReboots != 1 {
		t.Fatalf("a new monitor did not load the count: %+v", r.m.Health())
	}
	r.run(H, p.ResetAfter-p.Interval)
	if n, _ := r.store.Count(); n != 1 {
		t.Fatalf("count reset early: %d", n)
	}
	r.run(H, 2*p.Interval)
	if n, _ := r.store.Count(); n != 0 {
		t.Fatalf("count after %s healthy = %d, want 0", p.ResetAfter, n)
	}
	if r.last().AutoReboots != 0 {
		t.Fatalf("published auto_reboots = %d, want 0", r.last().AutoReboots)
	}
}

// Publishing only on change keeps a steady reader off the broker.
func TestMonitor_PublishesOnlyOnChange(t *testing.T) {
	r := newRig(t, DefaultHealthPolicy(), false)
	r.run(H, 10*time.Minute)
	if len(r.published) != 1 {
		t.Fatalf("published %d times for a steady reader, want 1", len(r.published))
	}
}

// The golden CloudServer fault rides the retained health: set on the tick that
// finds it, cleared on the tick after it is fixed.
func TestMonitor_CarriesCloudServerFault(t *testing.T) {
	r := newRig(t, DefaultHealthPolicy(), false)
	fault := "cleanSession is false; golden is true"
	r.m.deps.CloudServer = func() string { return fault }

	r.run(H, DefaultHealthPolicy().Interval)
	if got := r.last().CloudServerFault; got != fault {
		t.Fatalf("cloudserver_fault = %q, want %q", got, fault)
	}

	fault = ""
	r.run(H, DefaultHealthPolicy().Interval)
	if got := r.last().CloudServerFault; got != "" {
		t.Fatalf("cloudserver_fault after the fix = %q, want empty", got)
	}
}

func TestMonitor_NoCloudServerCheckLeavesFaultEmpty(t *testing.T) {
	r := newRig(t, DefaultHealthPolicy(), false)
	r.run(H, DefaultHealthPolicy().Interval)
	if got := r.last().CloudServerFault; got != "" {
		t.Fatalf("cloudserver_fault = %q, want empty with no check wired", got)
	}
}
