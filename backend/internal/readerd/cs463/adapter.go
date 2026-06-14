package cs463

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/trakrf/platform/backend/internal/readerrpc"
)

// readerOps is the minimal slice of CS463 reader operations the adapter drives.
// *Client satisfies it; tests inject a fake. Keeping it small keeps the adapter
// testable without an HTTP round-trip.
type readerOps interface {
	Login(ctx context.Context) (session, holderIP string, err error)
	Logout(ctx context.Context, session string) error
	ForceLogout(ctx context.Context) error
	GetActiveProfile(ctx context.Context, session string) (Profile, error)
	LoginServlet(ctx context.Context) error
	LogoutServlet(ctx context.Context) error
	SetProfilePower(ctx context.Context, profileID string, antennaCount int, enabledPorts []int, powers map[int]float64, profileFields map[string]string) error
	EnableEvent(ctx context.Context, session, eventID string, enable bool) error
}

// AdapterConfig holds the static facts the adapter needs about this reader.
type AdapterConfig struct {
	// AntennaCount is the number of physical antenna ports (CS463 = 4).
	AntennaCount int
	// EventID is the reader event bound to the active operation profile; it is
	// re-armed (disable then enable) after every profile write so inventory
	// resumes on the new configuration.
	EventID string
}

// Adapter implements readerd.Adapter for the CSL CS463 by mapping the neutral
// readerrpc contract onto the reader's HTTP/servlet operations.
type Adapter struct {
	ops readerOps
	cfg AdapterConfig
}

// Compile-time assertion: the transport Client satisfies readerOps. (That
// *Adapter satisfies readerd.Adapter is asserted in package readerd, where the
// daemon consumes it — asserting it here would import readerd and create a cycle
// once the daemon imports this package.)
var _ readerOps = (*Client)(nil)

// NewAdapter builds a CS463 adapter over the given reader operations.
func NewAdapter(ops readerOps, cfg AdapterConfig) *Adapter {
	return &Adapter{ops: ops, cfg: cfg}
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
			readerrpc.MethodGetConfig,
			readerrpc.MethodSetConfig,
			readerrpc.MethodGetStatus,
		},
		Unsupported: []string{
			readerrpc.MethodScanStart,
			readerrpc.MethodScanStop,
			readerrpc.MethodGpoSet,
			readerrpc.MethodReboot,
		},
	}, nil
}

// GetConfig reads the active profile and maps its per-port powers into the
// neutral config, sorted by antenna.
func (a *Adapter) GetConfig(ctx context.Context) (readerrpc.ReaderConfig, error) {
	session, holderIP, err := a.ops.Login(ctx)
	if err != nil {
		return readerrpc.ReaderConfig{}, err
	}
	if session == "" {
		return readerrpc.ReaderConfig{}, busyErr(holderIP)
	}
	defer func() { _ = a.ops.Logout(ctx, session) }()

	prof, err := a.ops.GetActiveProfile(ctx, session)
	if err != nil {
		return readerrpc.ReaderConfig{}, err
	}
	return readerrpc.ReaderConfig{TxPowerDBm: sortedPowers(prof.Powers)}, nil
}

// SetConfig validates, applies, re-arms, and verifies a power change. It is a
// read-modify-write against the active profile and returns pending_reload: the
// new configuration takes effect on the next inventory cycle.
func (a *Adapter) SetConfig(ctx context.Context, cfg readerrpc.ReaderConfig) (readerrpc.SetConfigResult, error) {
	// Validate the full request BEFORE touching the reader.
	for _, ap := range cfg.TxPowerDBm {
		if ap.Power < MinPowerDBm || ap.Power > MaxPowerDBm {
			return readerrpc.SetConfigResult{}, fmt.Errorf(
				"cs463: tx power %.1f dBm for antenna %d out of range [%.1f, %.1f]",
				ap.Power, ap.Antenna, MinPowerDBm, MaxPowerDBm)
		}
	}

	// The CS463 permits only ONE root login at a time, and the /API session_id and
	// the web-UI (JSESSIONID cookie) session BOTH consume that single slot. The
	// servlet write authenticates via the cookie, NOT the /API session, and an
	// /API session being held blocks the form login. So the three phases below are
	// strictly sequenced and each releases its session before the next phase opens
	// one. Do NOT introduce a function-level `defer Logout` that holds the /API
	// session across the Phase B form login (that is the bug this fixes).

	// --- Phase A: read the active profile via /API, then release the slot. ---
	session, holderIP, err := a.ops.Login(ctx)
	if err != nil {
		return readerrpc.SetConfigResult{}, err
	}
	if session == "" {
		return readerrpc.SetConfigResult{}, busyErr(holderIP)
	}

	prof, err := a.ops.GetActiveProfile(ctx, session)
	if err != nil {
		_ = a.ops.Logout(ctx, session)
		return readerrpc.SetConfigResult{}, err
	}
	// Release the /API session BEFORE the form login (single-session lock).
	_ = a.ops.Logout(ctx, session)

	enabledPorts := parseAntennaPorts(prof.Attrs["antenna_port"])

	// Merge: start from current powers, override with the requested antennas.
	powers := make(map[int]float64, a.cfg.AntennaCount)
	for port, pw := range prof.Powers {
		powers[port] = pw
	}
	for _, ap := range cfg.TxPowerDBm {
		powers[ap.Antenna] = ap.Power
	}

	// --- Phase B: write the profile via the web servlet (cookie auth), then
	// release the web session so the Phase C /API login can take the slot. ---
	if err := a.ops.LoginServlet(ctx); err != nil {
		return readerrpc.SetConfigResult{}, fmt.Errorf("cs463: servlet form login: %w", err)
	}
	if err := a.ops.SetProfilePower(ctx, prof.ID, a.cfg.AntennaCount, enabledPorts, powers, prof.Attrs); err != nil {
		_ = a.ops.LogoutServlet(ctx)
		return readerrpc.SetConfigResult{}, err
	}
	// Release the web session BEFORE the second /API login (single-session lock).
	_ = a.ops.LogoutServlet(ctx)

	// --- Phase C: verify the write, then re-arm the inventory event, via /API. ---
	session, holderIP, err = a.ops.Login(ctx)
	if err != nil {
		return readerrpc.SetConfigResult{}, err
	}
	if session == "" {
		return readerrpc.SetConfigResult{}, busyErr(holderIP)
	}

	// Verify the write did not wipe antenna enablement (#494 guard) BEFORE the
	// re-arm: the servlet can silently clear antenna_port, and we must read while
	// the profile is still active — the event re-arm briefly de-activates it during
	// the inventory reload (a post-re-arm read races that window and finds no
	// active profile).
	after, err := a.ops.GetActiveProfile(ctx, session)
	if err != nil {
		_ = a.ops.Logout(ctx, session)
		return readerrpc.SetConfigResult{}, fmt.Errorf("cs463: verify re-read: %w", err)
	}
	if after.Attrs["antenna_port"] != prof.Attrs["antenna_port"] {
		_ = a.ops.Logout(ctx, session)
		return readerrpc.SetConfigResult{}, fmt.Errorf(
			"cs463: antenna enablement changed after write (was %q, now %q) — refusing to report success",
			prof.Attrs["antenna_port"], after.Attrs["antenna_port"])
	}

	// Re-arm the inventory event so reading resumes on the new profile.
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

// --- helpers --------------------------------------------------------------

func busyErr(holderIP string) error {
	if holderIP != "" {
		return fmt.Errorf("cs463: reader is in use by %s", holderIP)
	}
	return fmt.Errorf("cs463: reader is in use")
}

// sortedPowers converts a port->dBm map into AntennaPower entries sorted by port.
func sortedPowers(powers map[int]float64) []readerrpc.AntennaPower {
	if len(powers) == 0 {
		return nil
	}
	out := make([]readerrpc.AntennaPower, 0, len(powers))
	for port, pw := range powers {
		out = append(out, readerrpc.AntennaPower{Antenna: port, Power: pw})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Antenna < out[j].Antenna })
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
