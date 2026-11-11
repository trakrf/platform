package cs463

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// fakeLines is an injectable GPOLines. It records every write into the
// fakeOps' shared ioOrder, so a test can assert how sysfs and /API writes
// interleave.
type fakeLines struct {
	f      *fakeOps
	setErr error

	mu    sync.Mutex
	value map[int]bool
}

func (l *fakeLines) Set(port int, on bool) error {
	l.f.mu.Lock()
	l.f.ioOrder = append(l.f.ioOrder, fmt.Sprintf("sysfs:%d:%v", port, on))
	l.f.mu.Unlock()
	if l.setErr != nil {
		return l.setErr
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.value == nil {
		l.value = map[int]bool{}
	}
	l.value[port] = on
	return nil
}

func (f *fakeOps) ioSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ioOrder...)
}

func newSafeAdapter(f *fakeOps, l *fakeLines, log *zerolog.Logger) *Adapter {
	return NewAdapter(f, AdapterConfig{AntennaCount: 4, GPO: l, Logger: log})
}

// Every OFF goes sysfs first (the horn stops now, whatever the vendor app is
// doing), then /API (so the vendor's single GPIO worker queue ends on off),
// then sysfs again: a vendor write queued just before ours lands ~30 ms after
// its ack, and the second sysfs write closes that window.
func TestGpoSet_OffDrivesSysfsAroundAPI(t *testing.T) {
	f := &fakeOps{}
	l := &fakeLines{f: f}
	a := newSafeAdapter(f, l, nil)

	if err := a.GpoSet(context.Background(), 2, false, 0); err != nil {
		t.Fatalf("GpoSet off: %v", err)
	}
	want := []string{"sysfs:2:false", "api:2:false", "sysfs:2:false"}
	if got := f.ioSnapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("io = %v, want %v", got, want)
	}
}

// ON stays on the vendor /API only. A wedged vendor app then fails the ON,
// which is the quiet direction; sysfs is never used to energise an output.
func TestGpoSet_OnNeverTouchesSysfs(t *testing.T) {
	f := &fakeOps{}
	l := &fakeLines{f: f}
	a := newSafeAdapter(f, l, nil)

	if err := a.GpoSet(context.Background(), 2, true, 5000); err != nil {
		t.Fatalf("GpoSet on: %v", err)
	}
	if got := f.ioSnapshot(); !reflect.DeepEqual(got, []string{"api:2:true"}) {
		t.Errorf("io = %v, want only the /API on", got)
	}
}

// The wedged-vendor-app case (cs463-212, 2026-07-25: /API answered 200 with an
// empty body and the pin never moved). The off still lands through sysfs, so
// the command succeeds.
func TestGpoSet_OffSucceedsThroughSysfsWhenAPIFails(t *testing.T) {
	f := &fakeOps{gpoOffErr: errors.New("cs463: empty body")}
	l := &fakeLines{f: f}
	a := newSafeAdapter(f, l, nil)

	if err := a.GpoSet(context.Background(), 2, false, 0); err != nil {
		t.Fatalf("GpoSet off with /API down = %v, want nil (sysfs drove it off)", err)
	}
	if l.value[2] {
		t.Error("port 2 still on")
	}
}

func TestGpoSet_OffFailsWhenBothPathsFail(t *testing.T) {
	f := &fakeOps{gpoOffErr: errors.New("api down")}
	l := &fakeLines{f: f, setErr: errors.New("gpio2 not exported")}
	a := newSafeAdapter(f, l, nil)

	err := a.GpoSet(context.Background(), 2, false, 0)
	if err == nil {
		t.Fatal("GpoSet off = nil, want an error when sysfs and /API both fail")
	}
	for _, s := range []string{"api down", "gpio2 not exported"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error %q does not name %q", err, s)
		}
	}
}

// Without a sysfs path (other hardware, or tests) the /API result decides.
func TestGpoSet_OffWithoutSysfsReportsAPIError(t *testing.T) {
	f := &fakeOps{gpoOffErr: errors.New("api down")}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4})
	if err := a.GpoSet(context.Background(), 2, false, 0); err == nil {
		t.Fatal("want the /API error when there is no sysfs path")
	}
}

// A pulse that expires while the vendor app is wedged must still release the
// port: the timer's off uses the same sysfs path.
func TestGpoSet_PulseReleaseUsesSysfs(t *testing.T) {
	f := &fakeOps{gpoOffErr: errors.New("cs463: empty body")}
	l := &fakeLines{f: f}
	a := newSafeAdapter(f, l, nil)

	if err := a.GpoSet(context.Background(), 2, true, 30); err != nil {
		t.Fatalf("GpoSet: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	want := []string{"api:2:true", "sysfs:2:false", "api:2:false", "sysfs:2:false"}
	if got := f.ioSnapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("io = %v, want %v", got, want)
	}
}

// OutputsOff is the start/stop safe state: every GPO port (1–4) driven off,
// and any pending release cancelled so it cannot fire later.
func TestOutputsOff_DrivesEveryPortAndCancelsTimers(t *testing.T) {
	f := &fakeOps{}
	l := &fakeLines{f: f}
	a := newSafeAdapter(f, l, nil)

	if err := a.GpoSet(context.Background(), 3, true, 60); err != nil {
		t.Fatalf("GpoSet: %v", err)
	}
	if err := a.OutputsOff(context.Background(), false); err != nil {
		t.Fatalf("OutputsOff: %v", err)
	}
	afterSweep := len(f.ioSnapshot())
	time.Sleep(150 * time.Millisecond) // past the 60 ms pulse

	got := f.ioSnapshot()
	if len(got) != afterSweep {
		t.Errorf("a cancelled release still fired: %v", got[afterSweep:])
	}
	for port := 1; port <= 4; port++ {
		for _, w := range []string{fmt.Sprintf("sysfs:%d:false", port), fmt.Sprintf("api:%d:false", port)} {
			if !contains(got, w) {
				t.Errorf("io %v is missing %s", got, w)
			}
		}
	}
	// A start-time sweep (final=false) leaves the adapter serving.
	if err := a.GpoSet(context.Background(), 1, true, 1000); err != nil {
		t.Errorf("GpoSet after a start sweep = %v, want nil", err)
	}
}

// The stop sweep is final: an ON that arrives after it (an RPC still in flight
// during shutdown) is refused and drives nothing. OFF still works.
func TestOutputsOff_FinalRefusesLaterOn(t *testing.T) {
	f := &fakeOps{}
	l := &fakeLines{f: f}
	a := newSafeAdapter(f, l, nil)

	if err := a.OutputsOff(context.Background(), true); err != nil {
		t.Fatalf("OutputsOff: %v", err)
	}
	before := len(f.ioSnapshot())
	if err := a.GpoSet(context.Background(), 2, true, 1000); err == nil {
		t.Error("GpoSet on after the stop sweep = nil, want a refusal")
	}
	if got := f.ioSnapshot(); len(got) != before {
		t.Errorf("a refused ON still wrote: %v", got[before:])
	}
	if err := a.GpoSet(context.Background(), 2, false, 0); err != nil {
		t.Errorf("GpoSet off after the stop sweep = %v, want nil", err)
	}
}

// One port failing must not stop the sweep reaching the rest, and every port's
// result is logged — the failure included.
func TestOutputsOff_LogsEachPortAndReportsFailure(t *testing.T) {
	var buf syncBuffer
	log := zerolog.New(&buf)
	f := &fakeOps{gpoOffErr: errors.New("api down")}
	l := &fakeLines{f: f, setErr: errors.New("not exported")}
	a := newSafeAdapter(f, l, &log)

	err := a.OutputsOff(context.Background(), true)
	if err == nil {
		t.Fatal("OutputsOff = nil, want an error when no port could be driven off")
	}
	out := buf.String()
	for port := 1; port <= 4; port++ {
		if !strings.Contains(out, fmt.Sprintf(`"port":%d`, port)) {
			t.Errorf("no log line for port %d:\n%s", port, out)
		}
	}
	if !strings.Contains(out, "not exported") || !strings.Contains(out, "api down") {
		t.Errorf("log does not carry both failure reasons:\n%s", out)
	}
}

func TestOutputsOff_LogsSuccessPerPort(t *testing.T) {
	var buf syncBuffer
	log := zerolog.New(&buf)
	f := &fakeOps{}
	a := newSafeAdapter(f, &fakeLines{f: f}, &log)

	if err := a.OutputsOff(context.Background(), false); err != nil {
		t.Fatalf("OutputsOff: %v", err)
	}
	if n := strings.Count(buf.String(), "gpo safe state"); n != 4 {
		t.Errorf("%d safe-state log lines, want 4:\n%s", n, buf.String())
	}
}

// --- sysfs ------------------------------------------------------------------

func fakeSysfs(t *testing.T, lines ...int) string {
	t.Helper()
	root := t.TempDir()
	for _, n := range lines {
		dir := filepath.Join(root, fmt.Sprintf("gpio%d", n))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "value"), []byte("1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func readValue(t *testing.T, root string, line int) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("gpio%d", line), "value"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

// The CS463 map, verified on cs463-212 and cs463-8405de: GPO1..4 are kernel
// lines 205, 2, 175, 176.
func TestSysfsGPO_WritesTheMappedLine(t *testing.T) {
	root := fakeSysfs(t, 205, 2, 175, 176)
	g := sysfsGPO{root: root, lines: cs463GPOLines}

	for port, line := range map[int]int{1: 205, 2: 2, 3: 175, 4: 176} {
		if err := g.Set(port, false); err != nil {
			t.Fatalf("Set(%d, false): %v", port, err)
		}
		if v := readValue(t, root, line); v != "0" {
			t.Errorf("gpio%d = %q after off, want 0", line, v)
		}
	}
	if err := g.Set(2, true); err != nil {
		t.Fatal(err)
	}
	if v := readValue(t, root, 2); v != "1" {
		t.Errorf("gpio2 = %q after on, want 1", v)
	}
}

func TestSysfsGPO_UnmappedPort(t *testing.T) {
	g := sysfsGPO{root: fakeSysfs(t), lines: cs463GPOLines}
	if err := g.Set(5, false); err == nil {
		t.Error("Set(5) = nil, want an unmapped-port error")
	}
}

// The daemon never exports a line itself — the vendor app owns that — so an
// unexported line is an error, not something to create.
func TestSysfsGPO_UnexportedLine(t *testing.T) {
	root := fakeSysfs(t) // nothing exported
	g := sysfsGPO{root: root, lines: cs463GPOLines}
	if err := g.Set(2, false); err == nil {
		t.Error("Set on an unexported line = nil, want an error")
	}
	if _, err := os.Stat(filepath.Join(root, "gpio2")); !os.IsNotExist(err) {
		t.Error("Set created the gpio2 directory; it must never export a line")
	}
}

// syncBuffer is a log sink a release goroutine can write while the test reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
