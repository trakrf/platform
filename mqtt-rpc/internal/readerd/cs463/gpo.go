package cs463

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// gpoReleaseTimeout bounds the deferred off-command of a one-shot GPO pulse. It
// is independent of the originating request's context, which is long gone by the
// time the pulse expires.
const gpoReleaseTimeout = 8 * time.Second

// gpoSafeStateTimeout bounds each port's /API off during the start and stop
// sweeps. The sysfs write does not wait on it: it lands first.
const gpoSafeStateTimeout = 2 * time.Second

// gpoPortCount is the CS463's GPO count (HD15 pins; see
// docs/cs463-reader-references.md).
const gpoPortCount = 4

// GPOLines drives the outputs below the vendor app. The vendor /API stops
// processing GPO commands when its web app wedges: on cs463-212 (2026-07-25)
// every directIOOutput answered 200 with an empty body and the pin never
// moved. The kernel's GPIO lines do not depend on that app, so every OFF also
// goes through them. ON never does — a wedged app failing an ON is the quiet
// direction.
type GPOLines interface {
	Set(port int, on bool) error
}

// cs463GPOLines maps GPO port → kernel GPIO line, verified over SSH on
// cs463-212 and cs463-8405de (and physically, GPO2 on cs463-212, 2026-11-08).
var cs463GPOLines = map[int]int{1: 205, 2: 2, 3: 175, 4: 176}

// SysfsGPO is the CS463's /sys/class/gpio path. Writing needs root, which is
// what mqtt-rpcd.service runs as.
func SysfsGPO() GPOLines {
	return sysfsGPO{root: "/sys/class/gpio", lines: cs463GPOLines}
}

type sysfsGPO struct {
	root  string
	lines map[int]int
}

// Set writes the line's value and reads it back. It never exports a line: the
// vendor app exports them at boot, and an unexported line is reported, not
// created.
func (g sysfsGPO) Set(port int, on bool) error {
	line, ok := g.lines[port]
	if !ok {
		return fmt.Errorf("cs463: gpo port %d has no gpio line", port)
	}
	path := filepath.Join(g.root, fmt.Sprintf("gpio%d", line), "value")
	want := "0"
	if on {
		want = "1"
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("cs463: gpio%d: %w", line, err)
	}
	_, werr := f.WriteString(want)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		return fmt.Errorf("cs463: write gpio%d: %w", line, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cs463: read back gpio%d: %w", line, err)
	}
	if got := strings.TrimSpace(string(b)); got != want {
		return fmt.Errorf("cs463: gpio%d reads %s after writing %s", line, got, want)
	}
	return nil
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

// GpoSet drives a general purpose output. It deliberately takes NO reader
// session: directIOOutput authenticates inline and bypasses the one-session-per-user
// lock, so an alarm fires even while an operator has the reader's web UI open.
//
// pulseMs > 0 with on==true arms a one-shot — the port is driven now and released
// after the delay by a timer HERE in the daemon, mirroring the Shelly toggle_after
// behaviour the geofence engine already relies on. The off edge is timed on the
// reader host and does not depend on a second message from the backend — but it
// lives in THIS process's memory, so it dies with the daemon. That is why the
// daemon drives every port off when it starts and when it stops (OutputsOff).
// The release runs on a background context so it survives the RPC's deadline;
// the caller gets its acknowledgement as soon as the port is energised rather
// than waiting out the pulse.
//
// Every OFF — explicit, a pulse's release, or the safe-state sweep — goes
// through driveOff, which also writes the kernel GPIO line, so it lands even
// while the vendor app is wedged.
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

	if !on {
		return a.driveOff(ctx, port)
	}
	if a.gpoClosed.Load() {
		return fmt.Errorf("cs463: gpo port %d: daemon is stopping; outputs are held off", port)
	}
	if err := a.ops.DirectIOOutput(ctx, port, true); err != nil {
		return err
	}
	if pulseMs <= 0 {
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
		p.timer = nil
		rctx, cancel := context.WithTimeout(context.Background(), gpoReleaseTimeout)
		defer cancel()
		if err := a.driveOff(rctx, port); err != nil {
			a.log.Error().Err(err).Int("port", port).Int("pulse_ms", pulseMs).Msg("gpo auto-off failed; the output may still be on")
			return
		}
		a.log.Info().Int("port", port).Int("pulse_ms", pulseMs).Msg("gpo auto-off")
	})
	p.timer = timer
	return nil
}

// driveOff turns one port off by every path there is, and succeeds when the
// output is known to be off. The caller holds the port's lock.
//
// Order matters. sysfs first: the output stops now, whatever the vendor app is
// doing. Then /API: the vendor applies GPO writes from a single worker queue
// ~30 ms after it acks them, so our off joins that queue behind any ON still
// in it, and the vendor's own state ends on off. Then sysfs again, which closes
// the window in which such a queued ON landed between the first two writes.
func (a *Adapter) driveOff(ctx context.Context, port int) error {
	if a.gpo == nil {
		return a.ops.DirectIOOutput(ctx, port, false)
	}
	firstErr := a.gpo.Set(port, false)
	apiErr := a.ops.DirectIOOutput(ctx, port, false)
	sysErr := a.gpo.Set(port, false)
	if sysErr == nil {
		if apiErr != nil {
			a.log.Warn().Err(apiErr).Int("port", port).Msg("gpo off: vendor /API failed; drove the line off through sysfs")
		}
		return nil
	}
	return fmt.Errorf("cs463: gpo port %d off failed: sysfs: %w; /API: %v", port, errors.Join(firstErr, sysErr), apiErr)
}

// OutputsOff is the safe state: it cancels every pending release and drives
// every GPO port off, logging each port's result. The daemon calls it when it
// starts, to clear a port left on by a crash, a kill -9 or the previous
// version, and when it stops, because a pending release dies with the process
// and a halted board on PoE holds its last GPO state (Loading Dock,
// 2026-11-08: a poweroff with the horn on left it on until the PoE was
// pulled).
//
// final is the stop sweep: every later ON is refused, so an RPC still in flight
// cannot re-energise an output after the sweep. A start sweep is not final.
//
// It tries every port whatever an earlier one did, and returns an error naming
// each port it could not drive off.
func (a *Adapter) OutputsOff(ctx context.Context, final bool) error {
	if final {
		a.gpoClosed.Store(true)
	}
	phase := "start"
	if final {
		phase = "stop"
	}
	var errs []error
	for _, port := range a.safeStatePorts() {
		p := a.gpoPort(port)
		p.mu.Lock()
		if p.timer != nil {
			p.timer.Stop()
			p.timer = nil
		}
		pctx, cancel := context.WithTimeout(ctx, gpoSafeStateTimeout)
		err := a.driveOff(pctx, port)
		cancel()
		p.mu.Unlock()
		if err != nil {
			a.log.Error().Err(err).Int("port", port).Str("phase", phase).Msg("gpo safe state: port NOT driven off")
			errs = append(errs, err)
			continue
		}
		a.log.Info().Int("port", port).Str("phase", phase).Msg("gpo safe state: port off")
	}
	return errors.Join(errs...)
}

// safeStatePorts is every physical GPO port plus any other port a command has
// touched, sorted.
func (a *Adapter) safeStatePorts() []int {
	seen := map[int]bool{}
	for port := 1; port <= gpoPortCount; port++ {
		seen[port] = true
	}
	a.gpoPortsMu.Lock()
	for port := range a.gpoPorts {
		seen[port] = true
	}
	a.gpoPortsMu.Unlock()
	out := make([]int, 0, len(seen))
	for port := range seen {
		out = append(out, port)
	}
	sort.Ints(out)
	return out
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
