package readerd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// fakeSystem records what a SystemdHost asked the system to do.
type fakeSystem struct {
	calls []string
	procs []int          // what cgroup.procs holds
	dieOn syscall.Signal // the signal that empties procs; 0 = nothing does
}

func newFakeSystemdHost(t *testing.T, fs *fakeSystem) *SystemdHost {
	t.Helper()
	dir := t.TempDir()
	procsFile := filepath.Join(dir, "cgroup.procs")
	if err := os.WriteFile(procsFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	log := zerolog.Nop()
	h := NewSystemdHost(&log)
	h.cgroupCandidates = []string{filepath.Join(dir, "missing", "cgroup.procs"), procsFile}
	h.run = func(_ context.Context, name string, args ...string) error {
		fs.calls = append(fs.calls, name+" "+strings.Join(args, " "))
		return nil
	}
	h.readProcs = func(string) ([]int, error) { return fs.procs, nil }
	h.kill = func(pid int, sig syscall.Signal) error {
		fs.calls = append(fs.calls, "kill "+sig.String())
		if sig == fs.dieOn {
			fs.procs = nil
		}
		return nil
	}
	h.sync = func() { fs.calls = append(fs.calls, "sync") }
	h.sleep = func(time.Duration) {}
	return h
}

func TestRestartReaderApp_StopsKillsLeftoversThenStarts(t *testing.T) {
	fs := &fakeSystem{procs: []int{492, 879}, dieOn: syscall.SIGTERM}
	h := newFakeSystemdHost(t, fs)

	if err := h.RestartReaderApp(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"systemctl stop embeddedglassfish.service",
		"kill terminated", "kill terminated",
		"systemctl start embeddedglassfish.service",
	}
	if strings.Join(fs.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %q\nwant    %q", fs.calls, want)
	}
}

// The vendor init script's stop kills by a pidfile that is not always there,
// and the unit is KillMode=process. Starting while the old java still runs
// gives two instances. A process that survives SIGKILL must stop the start, and
// the ladder escalates to a reboot.
func TestRestartReaderApp_SurvivorsBlockStart(t *testing.T) {
	fs := &fakeSystem{procs: []int{492}}
	h := newFakeSystemdHost(t, fs)

	err := h.RestartReaderApp(context.Background())
	if err == nil {
		t.Fatal("want an error when processes survive SIGKILL")
	}
	for _, c := range fs.calls {
		if strings.HasPrefix(c, "systemctl start") {
			t.Fatalf("started the app over a survivor: %q", fs.calls)
		}
	}
	if !contains(fs.calls, "kill killed") {
		t.Fatalf("never escalated to SIGKILL: %q", fs.calls)
	}
}

func TestRestartReaderApp_SigkillWhenTermIgnored(t *testing.T) {
	fs := &fakeSystem{procs: []int{492}, dieOn: syscall.SIGKILL}
	h := newFakeSystemdHost(t, fs)
	if err := h.RestartReaderApp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fs.calls[len(fs.calls)-1] != "systemctl start embeddedglassfish.service" {
		t.Fatalf("calls = %q, want a start last", fs.calls)
	}
}

// Without the cgroup the host cannot tell whether the old java exited, so it
// must not stop or start anything; the ladder goes to a reboot instead.
func TestRestartReaderApp_NoCgroupDoesNothing(t *testing.T) {
	fs := &fakeSystem{}
	h := newFakeSystemdHost(t, fs)
	h.cgroupCandidates = []string{filepath.Join(t.TempDir(), "nope", "cgroup.procs")}
	if err := h.RestartReaderApp(context.Background()); err == nil {
		t.Fatal("want an error when the cgroup cannot be found")
	}
	if len(fs.calls) != 0 {
		t.Fatalf("calls = %q, want none", fs.calls)
	}
}

func TestReboot_SyncsFirst(t *testing.T) {
	fs := &fakeSystem{}
	h := newFakeSystemdHost(t, fs)
	if err := h.Reboot(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fs.calls, "|") != "sync|systemctl reboot" {
		t.Fatalf("calls = %q, want sync then systemctl reboot", fs.calls)
	}
}

func TestReadCgroupProcs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cgroup.procs")
	if err := os.WriteFile(path, []byte("492\n879\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readCgroupProcs(path)
	if err != nil || len(got) != 2 || got[0] != 492 || got[1] != 879 {
		t.Fatalf("readCgroupProcs = %v, %v", got, err)
	}
	// A cgroup that is gone has no processes.
	got, err = readCgroupProcs(filepath.Join(t.TempDir(), "gone"))
	if err != nil || len(got) != 0 {
		t.Fatalf("missing file = %v, %v; want empty, nil", got, err)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
