package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultDir is the reader install directory.
const DefaultDir = "/opt/trakrf"

// DefaultGuardTimerUnit is the systemd timer that drives the guard's no-commit
// rollback. The updater refuses to run without it: an update with no armed
// rollback could strand a reader we cannot reach.
const DefaultGuardTimerUnit = "/etc/systemd/system/mqtt-rpcd-guard.timer"

const binName = "mqtt-rpcd"

// guardTimerWants is the link `systemctl enable` creates for the guard timer
// unit: its presence is what "enabled" means.
func guardTimerWants(unit string) string {
	return filepath.Join(filepath.Dir(unit), "timers.target.wants", filepath.Base(unit))
}

// Layout names every path of an install. On a reader:
//
//	/opt/trakrf/mqtt-rpcd                   -> versions/mqtt-rpcd-<active>  (symlink, systemd ExecStart)
//	/opt/trakrf/versions/mqtt-rpcd-<v>      one file per retained version
//	/opt/trakrf/update/{pending,last,...}   state shared with the guard
//	/opt/trakrf/mqtt-rpcd-guard             the rollback guard (never updated over the air)
//	/opt/trakrf/mqtt-rpcd.env               NEVER touched by the updater
type Layout struct{ Dir string }

func (l Layout) Link() string        { return filepath.Join(l.Dir, binName) }
func (l Layout) VersionsDir() string { return filepath.Join(l.Dir, "versions") }
func (l Layout) VersionPath(v string) string {
	return filepath.Join(l.VersionsDir(), binName+"-"+v)
}
func (l Layout) StateDir() string { return filepath.Join(l.Dir, "update") }
func (l Layout) Pending() string  { return filepath.Join(l.StateDir(), "pending") }
func (l Layout) Last() string     { return filepath.Join(l.StateDir(), "last") }
func (l Layout) Guard() string    { return filepath.Join(l.Dir, "mqtt-rpcd-guard") }

// linkTarget is the relative symlink target for version v.
func linkTarget(v string) string { return "versions/" + binName + "-" + v }

// SwapLink atomically repoints the active-version symlink at version v: a new
// link is created beside it and renamed over it, so a crash at any instant leaves
// either the old or the new link, never none.
func SwapLink(l Layout, v string) error {
	if !ValidVersion(v) {
		return fmt.Errorf("update: invalid version %q", v)
	}
	if _, err := os.Stat(l.VersionPath(v)); err != nil {
		return fmt.Errorf("update: version %s is not installed: %w", v, err)
	}
	tmp := filepath.Join(l.Dir, "."+binName+".link")
	_ = os.Remove(tmp)
	if err := os.Symlink(linkTarget(v), tmp); err != nil {
		return fmt.Errorf("update: create link: %w", err)
	}
	if err := os.Rename(tmp, l.Link()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("update: swap link: %w", err)
	}
	return syncDir(l.Dir)
}

// ActiveVersion reads the version the active symlink points at. A plain file at
// the link path is an install from before versioned binaries: it has nothing to
// roll back to, so it is an error rather than a guess.
func ActiveVersion(l Layout) (string, error) {
	target, err := os.Readlink(l.Link())
	if err != nil {
		return "", fmt.Errorf("update: %s is not a versioned symlink (re-run install.sh): %w", l.Link(), err)
	}
	v, ok := strings.CutPrefix(filepath.Base(target), binName+"-")
	if !ok || !ValidVersion(v) {
		return "", fmt.Errorf("update: unexpected link target %q", target)
	}
	return v, nil
}

// Prune keeps the newest keep versions (by mtime) plus every protected version,
// and removes the rest. Hidden temp files are CleanStale's.
func Prune(l Layout, keep int, protect ...string) error {
	entries, err := os.ReadDir(l.VersionsDir())
	if err != nil {
		return err
	}
	type ver struct {
		name string
		mod  int64
	}
	var vs []ver
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), binName+"-") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		vs = append(vs, ver{e.Name(), info.ModTime().UnixNano()})
	}
	sort.Slice(vs, func(i, j int) bool { return vs[i].mod > vs[j].mod })
	keepSet := map[string]bool{}
	for _, p := range protect {
		keepSet[binName+"-"+p] = true
	}
	var errs []error
	for i, v := range vs {
		if i < keep || keepSet[v.name] {
			continue
		}
		if err := os.Remove(filepath.Join(l.VersionsDir(), v.name)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// CleanStale removes the updater's own hidden temp files in versions/ —
// .mqtt-rpcd-<v>.download (this daemon and the guard's recover) and
// .mqtt-rpcd-<v>.upload (install.sh) — once they are older than olderThan. An
// interrupted transfer leaves one behind and nothing else ever removes it; the
// age keeps a transfer still in progress safe.
func CleanStale(l Layout, olderThan time.Duration) error {
	entries, err := os.ReadDir(l.VersionsDir())
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-olderThan)
	var errs []error
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, "."+binName+"-") ||
			(!strings.HasSuffix(n, ".download") && !strings.HasSuffix(n, ".upload")) {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(l.VersionsDir(), n)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// syncDir fsyncs a directory so a rename in it survives a power cut.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
