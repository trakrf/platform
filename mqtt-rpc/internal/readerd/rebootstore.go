package readerd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultRebootStorePath holds the automatic-reboot count on the reader's
// persistent root filesystem (/var/volatile is tmpfs; /opt is not).
const DefaultRebootStorePath = "/opt/trakrf/recovery/auto-reboots.json"

// RebootStore persists the automatic recovery reboots, so the cap that stops a
// boot loop survives the reboots it counts.
//
// It is a count of reboots since the API was last healthy for a sustained
// period, not a count per wall-clock window. The CS463's RTC has booted at
// 2019 before NTP corrected it. A timestamp window would then expire every
// earlier entry and allow unlimited reboots. The timestamps are kept for
// diagnosis only.
type RebootStore struct{ Path string }

type rebootFile struct {
	Reboots []string `json:"reboots"`
}

func (s RebootStore) load() (rebootFile, error) {
	var f rebootFile
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, fmt.Errorf("readerd: read reboot count: %w", err)
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("readerd: parse reboot count %s: %w", s.Path, err)
	}
	return f, nil
}

// Count returns the reboots recorded since the last Reset.
func (s RebootStore) Count() (int, error) {
	f, err := s.load()
	return len(f.Reboots), err
}

// Record adds one reboot. It must reach disk before the reboot does.
func (s RebootStore) Record(at time.Time) error {
	f, err := s.load()
	if err != nil {
		return err
	}
	f.Reboots = append(f.Reboots, at.UTC().Format(time.RFC3339))
	return s.write(f)
}

// Reset clears the count once the API has stayed healthy.
func (s RebootStore) Reset() error { return s.write(rebootFile{Reboots: []string{}}) }

// write replaces the file atomically and fsyncs both it and its directory.
func (s RebootStore) write(f rebootFile) error {
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("readerd: reboot count dir: %w", err)
	}
	b, err := json.Marshal(f)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".auto-reboots-*")
	if err != nil {
		return fmt.Errorf("readerd: write reboot count: %w", err)
	}
	defer os.Remove(tmp.Name()) // no-op after the rename
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("readerd: write reboot count: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("readerd: sync reboot count: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), s.Path); err != nil {
		return fmt.Errorf("readerd: replace reboot count: %w", err)
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
