package readerd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRebootStore_MissingFileCountsZero(t *testing.T) {
	s := RebootStore{Path: filepath.Join(t.TempDir(), "recovery", "auto-reboots.json")}
	n, err := s.Count()
	if err != nil || n != 0 {
		t.Fatalf("Count = %d, %v; want 0, nil", n, err)
	}
}

// The count is what stops a boot loop, so it must survive the reboot it allows.
func TestRebootStore_RecordSurvivesANewInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery", "auto-reboots.json")
	at := time.Date(2026, 11, 8, 17, 0, 0, 0, time.UTC)
	if err := (RebootStore{Path: path}).Record(at); err != nil {
		t.Fatal(err)
	}
	if err := (RebootStore{Path: path}).Record(at.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	n, err := RebootStore{Path: path}.Count()
	if err != nil || n != 2 {
		t.Fatalf("Count after two records = %d, %v; want 2", n, err)
	}
	if err := (RebootStore{Path: path}).Reset(); err != nil {
		t.Fatal(err)
	}
	if n, _ := (RebootStore{Path: path}).Count(); n != 0 {
		t.Fatalf("Count after Reset = %d, want 0", n)
	}
}

func TestRebootStore_CorruptFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auto-reboots.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (RebootStore{Path: path}).Count(); err == nil {
		t.Fatal("Count of a corrupt file: want an error")
	}
}
