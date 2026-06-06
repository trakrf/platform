package geofence

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

func TestPresence_FirstObserveFiresOnReObserveDoesNot(t *testing.T) {
	d := &fakeDriver{}
	p := newPresence(d, 0, NewFakeClock(time.Unix(0, 0)), zerolog.New(io.Discard))
	defer p.Close()
	dev := outputdevice.OutputDevice{ID: 5}

	if !p.observe(1, dev, time.Minute, "EPC", time.Unix(100, 0)) {
		t.Fatal("first observe must report ON edge")
	}
	if p.observe(1, dev, time.Minute, "EPC", time.Unix(110, 0)) {
		t.Fatal("re-observe while present must not report a new ON edge")
	}
}

func TestPresence_SweepFiresOffWhenLastMemberAgesOut(t *testing.T) {
	d := &fakeDriver{}
	clk := NewFakeClock(time.Unix(100, 0))
	p := newPresence(d, 0, clk, zerolog.New(io.Discard))
	defer p.Close()
	dev := outputdevice.OutputDevice{ID: 5}

	p.observe(1, dev, 10*time.Second, "EPC", time.Unix(100, 0))

	// 5s later: still within the 10s window, no OFF.
	clk.Advance(5 * time.Second)
	p.sweep(context.Background())
	if d.offCount() != 0 {
		t.Fatalf("member still present; expected 0 off, got %d", d.offCount())
	}

	// 20s after last seen: aged out -> exactly one OFF for device 5.
	clk.Advance(15 * time.Second)
	p.sweep(context.Background())
	if d.offCount() != 1 {
		t.Fatalf("aged-out member should drive one OFF, got %d", d.offCount())
	}
}

func TestPresence_TwoMembersOffOnlyWhenBothLeave(t *testing.T) {
	d := &fakeDriver{}
	clk := NewFakeClock(time.Unix(100, 0))
	p := newPresence(d, 0, clk, zerolog.New(io.Discard))
	defer p.Close()
	dev := outputdevice.OutputDevice{ID: 5}

	p.observe(1, dev, 10*time.Second, "A", time.Unix(100, 0))
	// B arrives 8s later; not an ON edge (already present), keeps the output alive.
	if p.observe(1, dev, 10*time.Second, "B", time.Unix(108, 0)) {
		t.Fatal("second member must not report a new ON edge")
	}

	// At t=112: A (last seen 100) aged out, B (last seen 108) still present.
	clk.Advance(12 * time.Second)
	p.sweep(context.Background())
	if d.offCount() != 0 {
		t.Fatalf("one member still present; expected 0 off, got %d", d.offCount())
	}

	// At t=120: B aged out too -> OFF.
	clk.Advance(8 * time.Second)
	p.sweep(context.Background())
	if d.offCount() != 1 {
		t.Fatalf("expected one off after both members left, got %d", d.offCount())
	}
}
