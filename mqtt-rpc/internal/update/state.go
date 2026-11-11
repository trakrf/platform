package update

// On-disk update state, shared with deploy/edge/mqtt-rpc/mqtt-rpcd-guard.
//
// Every file is `key=value` lines so the POSIX-sh guard can read it with sed.
// Values are versions (ValidVersion), reasons and RFC 3339 timestamps — never
// anything a shell would interpret.
//
//	update/pending   target=<v> from=<v> at=<ts>   an update is on probation
//	update/rollback.claim   pending, claimed by the guard while it rolls back
//	update/last      state=<s> target=<v> reason=<r> at=<ts>   last outcome
//	update/starts    integer, bumped by the guard on every unit start
//	update/ticks     integer, bumped by the guard's timer
//
// pending is claimed by rename: the daemon's commit and the guard's rollback each
// rename it away first, so exactly one of them acts.

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// ErrNotPending: there is no update on probation (none started, or the guard
// already claimed it).
var ErrNotPending = errors.New("update: no update pending")

// Pending is an update on probation.
type Pending struct {
	Target string // the version installed and now running
	From   string // the version to roll back to
	At     string
}

func nowStamp() string { return time.Now().UTC().Format(time.RFC3339) }

func readKV(path string) (map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			if _, dup := out[k]; !dup {
				out[k] = v
			}
		}
	}
	return out, sc.Err()
}

// writeKV writes keys in order via tmp + fsync + rename.
func writeKV(path string, kv [][2]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	for _, p := range kv {
		fmt.Fprintf(&buf, "%s=%s\n", p[0], p[1])
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf.Bytes()); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

// WritePending arms the guard for an update from p.From to p.Target and resets
// its counters.
func WritePending(l Layout, p Pending) error {
	if !ValidVersion(p.Target) || !ValidVersion(p.From) {
		return fmt.Errorf("update: invalid pending versions %q -> %q", p.From, p.Target)
	}
	if p.At == "" {
		p.At = nowStamp()
	}
	clearCounters(l)
	return writeKV(l.Pending(), [][2]string{{"target", p.Target}, {"from", p.From}, {"at", p.At}})
}

// ReadPending returns the update on probation, if any.
func ReadPending(l Layout) (Pending, bool, error) {
	kv, err := readKV(l.Pending())
	if errors.Is(err, os.ErrNotExist) {
		return Pending{}, false, nil
	}
	if err != nil {
		return Pending{}, false, err
	}
	return Pending{Target: kv["target"], From: kv["from"], At: kv["at"]}, true, nil
}

// CheckPending runs at daemon startup. It reports whether this process (running)
// is an update on probation. A pending marker naming another version means the
// updater died between arming the guard and swapping the link — the old binary
// is still the one running — so the marker is cleared rather than left for the
// guard to "roll back" a binary that was never replaced.
func CheckPending(l Layout, running string) (bool, error) {
	p, ok, err := ReadPending(l)
	if err != nil || !ok {
		return false, err
	}
	if p.Target == running {
		return true, nil
	}
	// The link already points at the target, yet this process reports another
	// version: the build was installed under a label that does not match its
	// compiled-in version. It is still the build on probation, and only the
	// guard may decide its fate — it cannot commit (Preflight sees the mismatch).
	if active, err := ActiveVersion(l); err == nil && active == p.Target {
		return false, nil
	}
	if err := os.Remove(l.Pending()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	clearCounters(l)
	return false, writeLast(l, readerrpc.UpdateStateFailed, p.Target, "swap_incomplete")
}

// probationArmed reports whether an update is still on probation on disk:
// pending, or the guard's claim on it while a rollback runs.
func probationArmed(l Layout) bool {
	for _, p := range []string{l.Pending(), filepath.Join(l.StateDir(), "rollback.claim")} {
		if _, err := os.Stat(p); err == nil || !errors.Is(err, os.ErrNotExist) {
			return true // present, or unreadable: assume still armed
		}
	}
	return false
}

// Commit ends running's probation: the guard will no longer roll it back.
// Returns ErrNotPending if the guard claimed the marker first.
func Commit(l Layout, running string) error {
	claim := l.Pending() + ".commit"
	if err := os.Rename(l.Pending(), claim); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotPending
		}
		return err
	}
	_ = os.Remove(claim)
	clearCounters(l)
	return writeLast(l, readerrpc.UpdateStateCommitted, running, "")
}

func writeLast(l Layout, state, target, reason string) error {
	return writeKV(l.Last(), [][2]string{{"state", state}, {"target", target}, {"reason", reason}, {"at", nowStamp()}})
}

// ReadLast returns the last recorded update outcome, if any. The guard's
// "reason" is reported as the UpdateInfo error.
func ReadLast(l Layout) (readerrpc.UpdateInfo, bool) {
	kv, err := readKV(l.Last())
	if err != nil {
		return readerrpc.UpdateInfo{}, false
	}
	return readerrpc.UpdateInfo{State: kv["state"], Target: kv["target"], Error: kv["reason"], At: kv["at"]}, true
}

func clearCounters(l Layout) {
	for _, f := range []string{"starts", "ticks"} {
		_ = os.Remove(filepath.Join(l.StateDir(), f))
	}
}
