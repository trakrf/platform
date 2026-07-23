package cs463

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// readerOps is the minimal slice of CS463 reader operations the adapter drives.
// *Client satisfies it; tests inject a fake. Keeping it small keeps the adapter
// testable without an HTTP round-trip.
type readerOps interface {
	Login(ctx context.Context) (session, holderIP string, err error)
	Logout(ctx context.Context, session string) error
	ForceLogout(ctx context.Context) error
	GetActiveProfile(ctx context.Context, session string) (Profile, error)
	ListEvent(ctx context.Context, session string) (map[string]EntityRow, error)
	ListTriggeringLogic(ctx context.Context, session string) (map[string]EntityRow, error)
	ModEvent(ctx context.Context, session string, p url.Values) error
	LoginServlet(ctx context.Context) error
	LogoutServlet(ctx context.Context) error
	SetProfilePower(ctx context.Context, profileID string, antennaCount int, enabledPorts []int, powers map[int]float64, profileFields map[string]string) error
	CreateProfile(ctx context.Context, session, profileID string, txPowerDBm float64) error
	EnableEvent(ctx context.Context, session, eventID string, enable bool) error
	DirectIOOutput(ctx context.Context, port int, on bool) error
}

// gpoReleaseTimeout bounds the deferred off-command of a one-shot GPO pulse. It
// is independent of the originating request's context, which is long gone by the
// time the pulse expires.
const gpoReleaseTimeout = 8 * time.Second

// AdapterConfig holds the static facts the adapter needs about this reader.
type AdapterConfig struct {
	// AntennaCount is the number of physical antenna ports (CS463 = 4).
	AntennaCount int
	// EventID is the reader event bound to the active operation profile; it is
	// re-armed (disable then enable) after every profile write so inventory
	// resumes on the new configuration.
	EventID string
}

// reconcileOps is the slice of reader operations the golden-config reconcile needs:
// the entity list/add/mod surface plus the session lifecycle and event re-arm. The
// transport *Client satisfies it; the adapter holds it separately from readerOps so
// SetOperProfile's fake does not have to implement the whole entity surface.
type reconcileOps interface {
	entityOps
	Login(ctx context.Context) (session, holderIP string, err error)
	Logout(ctx context.Context, session string) error
	EnableEvent(ctx context.Context, session, eventID string, enable bool) error
}

var _ reconcileOps = (*Client)(nil)

// Adapter implements readerd.Adapter for the CSL CS463 by mapping the neutral
// readerrpc contract onto the reader's HTTP/servlet operations.
type Adapter struct {
	ops readerOps
	rec reconcileOps // set when ops also satisfies reconcileOps (the real *Client)
	cfg AdapterConfig

	// gpoPortsMu guards ONLY the gpoPorts map's lookup/lazy-create step — it is
	// never held across a reader HTTP call. Each port gets its OWN portGPO
	// (and its own mutex), so a slow or hung reader call driving one GPO port
	// can never block Gpo.Set on any OTHER port. See gpoPort and GpoSet.
	gpoPortsMu sync.Mutex
	gpoPorts   map[int]*portGPO
}

// portGPO holds one GPO port's pending-release state. At most one timer is
// ever armed per port; mu serializes that port's on/off writes against its
// own release callback (see GpoSet) so the two can never land out of order.
// This is per-port BY DESIGN: same-port GpoSet calls (and their release)
// must serialize — you never want two conflicting writes to one physical
// output in flight at once — but DIFFERENT ports must not contend with each
// other, which is why this lock is not the adapter-wide gpoPortsMu.
type portGPO struct {
	mu    sync.Mutex
	timer *time.Timer
}

// Compile-time assertion: the transport Client satisfies readerOps. (That
// *Adapter satisfies readerd.Adapter is asserted in package readerd, where the
// daemon consumes it — asserting it here would import readerd and create a cycle
// once the daemon imports this package.)
var _ readerOps = (*Client)(nil)

// NewAdapter builds a CS463 adapter over the given reader operations. When ops also
// satisfies reconcileOps (the real *Client does), the adapter can run the golden
// config reconcile; SetOperProfile-only fakes that do not are simply not reconcilable.
func NewAdapter(ops readerOps, cfg AdapterConfig) *Adapter {
	a := &Adapter{ops: ops, cfg: cfg, gpoPorts: make(map[int]*portGPO)}
	if r, ok := ops.(reconcileOps); ok {
		a.rec = r
	}
	return a
}

// Reconcile converges the reader to the golden TrakRF mqtt-rpc entities and starts
// inventory. It runs in a single /API login window (all entity writes are /API,
// unlike the SetOperProfile servlet path): verify the pre-created CloudServer + Operation
// Profile exist, list-then-add-or-mod the four owned entities, then re-arm the golden
// event. A safe function-level defer Logout is fine here precisely because no servlet
// form login is taken inside this window (contrast SetOperProfile's three-phase dance).
//
// The event is re-armed UNCONDITIONALLY (not only when config changed) — a defensive
// measure, because the CS463 does not RELIABLY auto-start inventory: an event with
// enable=true in config sometimes publishes nothing until a disable→enable cycle kicks
// the inventory engine (operator-confirmed "faith cure"; a bare enable(true) is a
// no-op). A clean boot often does auto-start, but the daemon can't tell, so it arms on
// every startup to guarantee reads. Cost is one inventory cycle if reads were already
// flowing. (A future on-demand Reader.Reconcile RPC run against an already-reading
// reader should gate the re-arm on whether config changed, to avoid that blip.)
func (a *Adapter) Reconcile(ctx context.Context) error {
	if a.rec == nil {
		return fmt.Errorf("cs463: reconcile not supported by these reader ops")
	}
	// Ensure our Operation Profile exists (create-if-absent with an antenna-1 default)
	// (a.ops is always set by NewAdapter alongside a.rec.)
	// in its own session dance before the entity reconcile.
	if err := a.ensureProfile(ctx); err != nil {
		return err
	}

	session, holderIP, err := a.rec.Login(ctx)
	if err != nil {
		return err
	}
	if session == "" {
		return busyErr(holderIP)
	}
	defer func() { _ = a.rec.Logout(ctx, session) }()

	if err := verifyServer(ctx, session, a.rec); err != nil {
		return err
	}
	if _, err := reconcileGolden(ctx, session, a.rec, a.cfg.AntennaCount); err != nil {
		return err
	}
	// Always re-arm: start (or restart) inventory on the golden event.
	if err := a.rec.EnableEvent(ctx, session, NameEvent, false); err != nil {
		return fmt.Errorf("cs463: reconcile re-arm disable: %w", err)
	}
	if err := a.rec.EnableEvent(ctx, session, NameEvent, true); err != nil {
		return fmt.Errorf("cs463: reconcile re-arm enable: %w", err)
	}
	return nil
}

// ensureProfile creates the golden Operation Profile if it is absent, with a default
// antenna-1-only @ DefaultProfileTxPowerDBm config (and dwell=golden on every slot).
// If the profile already exists it is LEFT UNTOUCHED — the operator owns its antenna/
// TX-power tuning via Reader.SetOperProfile; the daemon never clobbers it. Creation
// needs the servlet to actually enable the antenna (setOperProfile can't on this
// firmware), so this runs its own /API-then-servlet session sequence (each released
// before the next, per the single-session lock).
func (a *Adapter) ensureProfile(ctx context.Context) error {
	// Phase A (/API): does our profile already exist?
	session, holderIP, err := a.ops.Login(ctx)
	if err != nil {
		return err
	}
	if session == "" {
		return busyErr(holderIP)
	}
	profiles, err := a.rec.ListProfileIDs(ctx, session)
	if err != nil {
		_ = a.ops.Logout(ctx, session)
		return fmt.Errorf("cs463: list profiles: %w", err)
	}
	if profiles[NameProfile] {
		_ = a.ops.Logout(ctx, session) // exists — hands off (operator owns tuning)
		return nil
	}
	if err := a.ops.CreateProfile(ctx, session, NameProfile, DefaultProfileTxPowerDBm); err != nil {
		_ = a.ops.Logout(ctx, session)
		return fmt.Errorf("cs463: create profile: %w", err)
	}
	_ = a.ops.Logout(ctx, session) // release /API before the servlet login

	// Phase B (servlet): actually enable antenna 1 (setOperProfile cannot).
	if err := a.ops.LoginServlet(ctx); err != nil {
		return fmt.Errorf("cs463: servlet login for profile: %w", err)
	}
	err = a.ops.SetProfilePower(ctx, NameProfile, a.cfg.AntennaCount,
		[]int{1}, map[int]float64{1: DefaultProfileTxPowerDBm}, map[string]string{})
	_ = a.ops.LogoutServlet(ctx)
	if err != nil {
		return fmt.Errorf("cs463: enable antenna 1 on profile: %w", err)
	}
	return nil
}

// GetCapabilities reports the static CS463 control surface. No reader round-trip.
func (a *Adapter) GetCapabilities(ctx context.Context) (readerrpc.Capabilities, error) {
	return readerrpc.Capabilities{
		ContractVersion: readerrpc.ContractVersion,
		ReaderModel:     "csl_cs463",
		Antennas:        a.cfg.AntennaCount,
		TxPower: readerrpc.TxPowerCap{
			MinDBm:     MinPowerDBm,
			MaxDBm:     MaxPowerDBm,
			PerAntenna: true,
		},
		Supports: []string{
			readerrpc.MethodGetCapabilities,
			readerrpc.MethodGetOperProfile,
			readerrpc.MethodSetOperProfile,
			readerrpc.MethodGetStatus,
			readerrpc.MethodGpoSet,
		},
		Unsupported: []string{
			readerrpc.MethodScanStart,
			readerrpc.MethodScanStop,
			readerrpc.MethodReboot,
		},
	}, nil
}

// GetOperProfile reads the active profile and the golden event/trigger entities,
// mapping per-antenna enablement + power and the read-only golden knobs into the
// neutral config. force force-logs-out a held single session first.
func (a *Adapter) GetOperProfile(ctx context.Context, force bool) (readerrpc.ReaderConfig, error) {
	session, err := a.login(ctx, force)
	if err != nil {
		return readerrpc.ReaderConfig{}, err
	}
	defer func() { _ = a.ops.Logout(ctx, session) }()

	prof, err := a.ops.GetActiveProfile(ctx, session)
	if err != nil {
		return readerrpc.ReaderConfig{}, err
	}

	cfg := readerrpc.ReaderConfig{Antennas: antennaConfigs(prof, a.cfg.AntennaCount)}
	if dwell, ok := firstDwellMs(prof.Attrs, a.cfg.AntennaCount); ok {
		cfg.DwellMs = &dwell
	}
	if events, err := a.ops.ListEvent(ctx, session); err == nil {
		if row, ok := events[NameEvent]; ok {
			if v, ok := atoiOK(row["duplicateEliminationWindow"]); ok {
				cfg.DedupWindowMs = &v
			}
			if raw := row["antennaDifferentiation"]; raw != "" {
				b := strings.EqualFold(raw, "true")
				cfg.AntennaDifferentiation = &b
			}
		}
	}
	if logics, err := a.ops.ListTriggeringLogic(ctx, session); err == nil {
		if row, ok := logics[NameTrigger]; ok {
			if v, ok := atoiOK(row["logic"]); ok {
				cfg.RSSIGateDBm = &v
			}
		}
	}
	return cfg, nil
}

// SetOperProfile applies a (partial) reader config: per-antenna enablement +
// power and reader-wide dwell via the servlet RMW; the event read-timing knobs
// dedup/antDiff via modEvent. A nil/empty field is left unchanged. It returns
// pending_reload (effective next inventory cycle). force force-logs-out a held
// single session first.
//
// The CS463 permits only ONE root login at a time; the /API session_id and the
// web-UI cookie session both consume that slot, so phases are strictly sequenced
// and each releases its session before the next opens one. Do NOT hold the /API
// session across the Phase B form login.
func (a *Adapter) SetOperProfile(ctx context.Context, cfg readerrpc.ReaderConfig, force bool) (readerrpc.SetConfigResult, error) {
	for _, ac := range cfg.Antennas {
		if ac.Enabled && (ac.PowerDBm < MinPowerDBm || ac.PowerDBm > MaxPowerDBm) {
			return readerrpc.SetConfigResult{}, fmt.Errorf(
				"cs463: tx power %.1f dBm for antenna %d out of range [%.1f, %.1f]",
				ac.PowerDBm, ac.Antenna, MinPowerDBm, MaxPowerDBm)
		}
	}
	if cfg.DwellMs != nil && *cfg.DwellMs <= 0 {
		return readerrpc.SetConfigResult{}, fmt.Errorf("cs463: dwell_ms must be > 0")
	}
	if cfg.DedupWindowMs != nil && *cfg.DedupWindowMs <= 0 {
		return readerrpc.SetConfigResult{}, fmt.Errorf("cs463: dedup_window_ms must be > 0")
	}

	needProfile := len(cfg.Antennas) > 0 || cfg.DwellMs != nil
	needEvent := cfg.DedupWindowMs != nil || cfg.AntennaDifferentiation != nil
	if !needProfile && !needEvent {
		return readerrpc.SetConfigResult{}, fmt.Errorf("cs463: SetOperProfile: nothing to set")
	}

	var enabledPorts []int

	if needProfile {
		// --- Phase A: read the active profile via /API, then release the slot. ---
		session, err := a.login(ctx, force)
		if err != nil {
			return readerrpc.SetConfigResult{}, err
		}
		prof, err := a.ops.GetActiveProfile(ctx, session)
		if err != nil {
			_ = a.ops.Logout(ctx, session)
			return readerrpc.SetConfigResult{}, err
		}
		_ = a.ops.Logout(ctx, session)

		// Merge requested enablement + power over the reader's current profile.
		enabled := make(map[int]bool)
		for _, p := range parseAntennaPorts(prof.Attrs["antenna_port"]) {
			enabled[p] = true
		}
		powers := make(map[int]float64, a.cfg.AntennaCount)
		for port, pw := range prof.Powers {
			powers[port] = pw
		}
		for _, ac := range cfg.Antennas {
			enabled[ac.Antenna] = ac.Enabled
			powers[ac.Antenna] = ac.PowerDBm
		}
		enabledPorts = sortedEnabledPorts(enabled)

		// Dwell is applied uniformly to all ports via the servlet's profileFields
		// (SetProfilePower sources dwell from dwellTime{port}); clone so we don't
		// mutate the read profile.
		fields := prof.Attrs
		if cfg.DwellMs != nil {
			fields = cloneAttrs(prof.Attrs)
			for port := 1; port <= a.cfg.AntennaCount; port++ {
				fields["dwellTime"+strconv.Itoa(port)] = strconv.Itoa(*cfg.DwellMs)
			}
		}

		// --- Phase B: write via the web servlet (cookie auth), then release. ---
		if err := a.ops.LoginServlet(ctx); err != nil {
			return readerrpc.SetConfigResult{}, fmt.Errorf("cs463: servlet form login: %w", err)
		}
		if err := a.ops.SetProfilePower(ctx, prof.ID, a.cfg.AntennaCount, enabledPorts, powers, fields); err != nil {
			_ = a.ops.LogoutServlet(ctx)
			return readerrpc.SetConfigResult{}, err
		}
		_ = a.ops.LogoutServlet(ctx)
	}

	// --- Phase C (/API): verify profile write, push event knobs, re-arm. ---
	session, err := a.login(ctx, force)
	if err != nil {
		return readerrpc.SetConfigResult{}, err
	}
	if needProfile {
		after, err := a.ops.GetActiveProfile(ctx, session)
		if err != nil {
			_ = a.ops.Logout(ctx, session)
			return readerrpc.SetConfigResult{}, fmt.Errorf("cs463: verify re-read: %w", err)
		}
		gotPorts := parseAntennaPorts(after.Attrs["antenna_port"])
		if !sameInts(gotPorts, enabledPorts) {
			_ = a.ops.Logout(ctx, session)
			return readerrpc.SetConfigResult{}, fmt.Errorf(
				"cs463: antenna enablement not applied (wanted %v, reader has %v) — refusing to report success",
				enabledPorts, gotPorts)
		}
	}
	if needEvent {
		// modEvent re-writes the golden event with the customer's dedup/antDiff
		// overrides (other event fields stay golden — the daemon owns the event
		// identity). The golden reconcile no longer re-asserts these two fields, so
		// the edit survives a daemon restart (TRA-1002 coexistence).
		p := goldenEventParams()
		if cfg.DedupWindowMs != nil {
			p.Set("duplicateEliminationWindow", strconv.Itoa(*cfg.DedupWindowMs))
		}
		if cfg.AntennaDifferentiation != nil {
			p.Set("antennaDifferentiation", strconv.FormatBool(*cfg.AntennaDifferentiation))
		}
		if err := a.ops.ModEvent(ctx, session, p); err != nil {
			_ = a.ops.Logout(ctx, session)
			return readerrpc.SetConfigResult{}, fmt.Errorf("cs463: write event knobs: %w", err)
		}
	}
	// Re-arm the inventory event so changes take effect on the next cycle.
	if err := a.ops.EnableEvent(ctx, session, a.cfg.EventID, false); err != nil {
		_ = a.ops.Logout(ctx, session)
		return readerrpc.SetConfigResult{}, fmt.Errorf("cs463: re-arm disable: %w", err)
	}
	if err := a.ops.EnableEvent(ctx, session, a.cfg.EventID, true); err != nil {
		_ = a.ops.Logout(ctx, session)
		return readerrpc.SetConfigResult{}, fmt.Errorf("cs463: re-arm enable: %w", err)
	}
	_ = a.ops.Logout(ctx, session)

	return readerrpc.SetConfigResult{
		Applied:     readerrpc.AppliedPendingReload,
		EffectiveAt: "next_inventory_cycle",
	}, nil
}

// GetStatus reports liveness and whether the active profile has any enabled
// antenna (a proxy for "reading").
func (a *Adapter) GetStatus(ctx context.Context) (readerrpc.Status, error) {
	session, holderIP, err := a.ops.Login(ctx)
	if err != nil {
		return readerrpc.Status{}, err
	}
	if session == "" {
		return readerrpc.Status{}, busyErr(holderIP)
	}
	defer func() { _ = a.ops.Logout(ctx, session) }()

	prof, err := a.ops.GetActiveProfile(ctx, session)
	if err != nil {
		return readerrpc.Status{}, err
	}
	return readerrpc.Status{
		Online:        true,
		Reading:       len(parseAntennaPorts(prof.Attrs["antenna_port"])) > 0,
		ActiveProfile: prof.ID,
	}, nil
}

// GpoSet drives a general purpose output. It deliberately takes NO reader
// session: directIOOutput authenticates inline and bypasses the single-root-session
// lock, so an alarm fires even while an operator has the reader's web UI open.
//
// pulseMs > 0 with on==true arms a one-shot — the port is driven now and released
// after the delay by a timer HERE in the daemon, mirroring the Shelly toggle_after
// behaviour the geofence engine already relies on. The off edge is timed on the
// reader host and does not depend on a second message from the backend — but it
// lives in THIS process's memory, not the reader's silicon, so it is lost if the
// daemon itself restarts mid-pulse (no persistence, no startup safe-state drive;
// see the TRA-1028 follow-up note). The release runs on a background context so
// it survives the RPC's deadline; the caller gets its acknowledgement as soon as
// the port is energised rather than waiting out the pulse.
//
// Every call for a port supersedes whatever that port was doing: any pending
// release timer is stopped before the new command drives the port, and a fresh
// timer (only for on&&pulseMs>0) replaces it. This mirrors Shelly's toggle_after
// REPLACE semantics — two tags exiting within one auto_off window must not race
// (see TRA-1028): the first pulse's timer must never cut the second, later
// command short, AND a stale release that already started must never land its
// OFF write after a superseding command's write (the identity check in the
// release callback below happens BEFORE that write, under the same per-port
// lock a superseding GpoSet call takes, precisely to close that ordering gap).
//
// Locking is PER PORT (portGPO.mu, via gpoPort), not adapter-wide: a slow or
// hung reader call driving one GPO port must never block Gpo.Set on a
// different port (a multi-zone alarm must keep real-time delivery on every
// port even if one is stuck). Same-port calls DO serialize on that port's
// lock, including across the reader HTTP round-trip — that's intentional,
// since two conflicting writes to the SAME physical output must never be in
// flight at once.
func (a *Adapter) GpoSet(ctx context.Context, port int, on bool, pulseMs int) error {
	p := a.gpoPort(port)
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}

	if err := a.ops.DirectIOOutput(ctx, port, on); err != nil {
		return err
	}
	if !on || pulseMs <= 0 {
		return nil
	}

	// Arm the fresh release timer and publish it to p.timer BEFORE releasing
	// the lock. This is deliberate, not incidental: time.AfterFunc's callback
	// runs on its own goroutine and could in principle fire the instant the
	// timer is created (e.g. a very short pulse under scheduler pressure). By
	// holding p.mu across both the timer's creation and the `timer` closure
	// variable's assignment (this whole function holds it, via the defer
	// above), and having the callback's FIRST action be to take the same
	// lock, the mutex's Unlock-before-Lock ordering guarantees the callback
	// can never observe `timer` (or p.timer) in a partially-written state —
	// it simply blocks until this critical section finishes.
	var timer *time.Timer
	timer = time.AfterFunc(time.Duration(pulseMs)*time.Millisecond, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		// Identity check BEFORE the OFF write, under the SAME lock a
		// superseding GpoSet call for this port also takes. This is the
		// crux of the TRA-1028 review fix: the naive version issued the OFF
		// write first and checked identity only for its own map cleanup
		// afterwards, so a slow/in-flight OFF could still land AFTER a
		// superseding GpoSet's ON — leaving the port physically off with
		// nothing to correct it. Checking first and gating the write on it
		// means exactly one of two orderings can happen, and both are
		// correct: (a) this callback gets the lock first, finds itself
		// still current, and writes OFF — a superseding GpoSet then blocks
		// on p.mu until that OFF completes, and writes its own ON last; or
		// (b) a superseding GpoSet gets the lock first, stops/replaces this
		// timer (p.timer != timer) and writes its ON — this callback then
		// gets the lock, sees it has been superseded, and writes NOTHING.
		if p.timer != timer {
			return
		}
		rctx, cancel := context.WithTimeout(context.Background(), gpoReleaseTimeout)
		// Best-effort: a failed release leaves the output latched on, which is a
		// loud, visible failure rather than a silent one. Nothing useful to do here
		// but let the next command correct it.
		_ = a.ops.DirectIOOutput(rctx, port, false)
		cancel()
		p.timer = nil
	})
	p.timer = timer
	return nil
}

// gpoPort returns (lazily creating) the per-port GPO state for port. The
// gpoPortsMu lock is held only for this map lookup/insert step — never
// across a reader HTTP call — so unrelated ports never contend with each
// other over it; all actual serialization for a port's writes happens on
// that port's own portGPO.mu (see GpoSet).
func (a *Adapter) gpoPort(port int) *portGPO {
	a.gpoPortsMu.Lock()
	defer a.gpoPortsMu.Unlock()
	p := a.gpoPorts[port]
	if p == nil {
		p = &portGPO{}
		a.gpoPorts[port] = p
	}
	return p
}

// --- helpers --------------------------------------------------------------

func busyErr(holderIP string) error {
	return &readerrpc.BusyError{HeldBy: holderIP}
}

// login opens an /API session, force-logging-out a held session first when force
// is set. On a busy reader without force it returns a *readerrpc.BusyError.
func (a *Adapter) login(ctx context.Context, force bool) (string, error) {
	session, holderIP, err := a.ops.Login(ctx)
	if err != nil {
		return "", err
	}
	if session != "" {
		return session, nil
	}
	if !force {
		return "", busyErr(holderIP)
	}
	if err := a.ops.ForceLogout(ctx); err != nil {
		return "", fmt.Errorf("cs463: force logout: %w", err)
	}
	session, holderIP, err = a.ops.Login(ctx)
	if err != nil {
		return "", err
	}
	if session == "" {
		return "", busyErr(holderIP)
	}
	return session, nil
}

// antennaConfigs builds per-antenna enabled+power for ports 1..count, sorted by
// antenna. Enablement is driven by the profile's antenna_port set; power from the
// profile's per-port powers (0 when absent).
func antennaConfigs(prof Profile, count int) []readerrpc.AntennaConfig {
	enabled := make(map[int]bool)
	for _, p := range parseAntennaPorts(prof.Attrs["antenna_port"]) {
		enabled[p] = true
	}
	if count <= 0 {
		for p := range prof.Powers {
			if p > count {
				count = p
			}
		}
		for p := range enabled {
			if p > count {
				count = p
			}
		}
	}
	out := make([]readerrpc.AntennaConfig, 0, count)
	for port := 1; port <= count; port++ {
		out = append(out, readerrpc.AntennaConfig{
			Antenna:  port,
			Enabled:  enabled[port],
			PowerDBm: prof.Powers[port],
		})
	}
	return out
}

// firstDwellMs returns the dwellTime of the lowest port that has one (golden sets
// all slots equal, so the first is representative).
func firstDwellMs(attrs map[string]string, count int) (int, bool) {
	if count <= 0 {
		count = 16
	}
	for port := 1; port <= count; port++ {
		if v, ok := attrs["dwellTime"+strconv.Itoa(port)]; ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

func atoiOK(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return n, true
}

func sortedEnabledPorts(enabled map[int]bool) []int {
	out := []int{}
	for port, on := range enabled {
		if on {
			out = append(out, port)
		}
	}
	sort.Ints(out)
	return out
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func cloneAttrs(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// parseAntennaPorts parses the CS463 "antenna_port" attribute (e.g. "1,2,4")
// into a sorted-by-appearance list of port numbers. Empty/blank yields nil.
func parseAntennaPorts(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			out = append(out, n)
		}
	}
	return out
}
