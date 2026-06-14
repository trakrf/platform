package poweragent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/poweragent/csl"
	"github.com/trakrf/platform/backend/internal/readerpower"
)

type fakeApplier struct {
	res        csl.Result
	err        error
	gotPowers  map[int]float64
	gotForce   bool
}

func (f *fakeApplier) Apply(_ context.Context, powers map[int]float64, force bool) (csl.Result, error) {
	f.gotPowers = powers
	f.gotForce = force
	return f.res, f.err
}

func newTestAgent(fa *fakeApplier) (*Agent, *reader) {
	a := &Agent{
		log:     zerolog.Nop(),
		readers: map[string]*reader{},
		now:     func() time.Time { return time.Unix(0, 0) },
	}
	rdr := &reader{cfg: ReaderConfig{PublishTopic: "demo/cs463-212"}, client: fa}
	a.readers[readerpower.CommandTopic(rdr.cfg.PublishTopic)] = rdr
	return a, rdr
}

func TestHandleCommand_OK(t *testing.T) {
	fa := &fakeApplier{res: csl.Result{ActiveProfile: "TrakRF", Powers: map[int]float64{1: 22.5, 2: 30}}}
	a, rdr := newTestAgent(fa)

	st := a.handleCommand(context.Background(), rdr, readerpower.Command{
		RequestID: "r1",
		Powers:    map[string]float64{"1": 22.5},
	})

	if st.Status != readerpower.StatusOK {
		t.Fatalf("status = %q, want ok", st.Status)
	}
	if st.RequestID != "r1" || st.ActiveProfile != "TrakRF" {
		t.Fatalf("unexpected state: %+v", st)
	}
	if st.Powers["1"] != 22.5 || st.Powers["2"] != 30 {
		t.Fatalf("powers = %v", st.Powers)
	}
	if fa.gotPowers[1] != 22.5 {
		t.Fatalf("applier got powers %v, want port1=22.5", fa.gotPowers)
	}
}

func TestHandleCommand_Busy(t *testing.T) {
	fa := &fakeApplier{res: csl.Result{Busy: true, HolderIP: "10.0.0.9"}}
	a, rdr := newTestAgent(fa)

	st := a.handleCommand(context.Background(), rdr, readerpower.Command{RequestID: "r2", Powers: map[string]float64{"1": 20}})
	if st.Status != readerpower.StatusBusy {
		t.Fatalf("status = %q, want busy", st.Status)
	}
	if st.HolderIP != "10.0.0.9" {
		t.Fatalf("holder = %q", st.HolderIP)
	}
}

func TestHandleCommand_Force(t *testing.T) {
	fa := &fakeApplier{res: csl.Result{ActiveProfile: "P", Powers: map[int]float64{1: 25}}}
	a, rdr := newTestAgent(fa)
	a.handleCommand(context.Background(), rdr, readerpower.Command{RequestID: "r3", Powers: map[string]float64{"1": 25}, Force: true})
	if !fa.gotForce {
		t.Fatalf("force flag not passed through to applier")
	}
}

func TestHandleCommand_ApplyError(t *testing.T) {
	fa := &fakeApplier{err: errors.New("reader unreachable")}
	a, rdr := newTestAgent(fa)
	st := a.handleCommand(context.Background(), rdr, readerpower.Command{RequestID: "r4", Powers: map[string]float64{"1": 20}})
	if st.Status != readerpower.StatusError {
		t.Fatalf("status = %q, want error", st.Status)
	}
	if st.Error == "" {
		t.Fatalf("expected error message")
	}
}

func TestHandleCommand_BadPortKey(t *testing.T) {
	fa := &fakeApplier{}
	a, rdr := newTestAgent(fa)
	st := a.handleCommand(context.Background(), rdr, readerpower.Command{RequestID: "r5", Powers: map[string]float64{"notaport": 20}})
	if st.Status != readerpower.StatusError {
		t.Fatalf("status = %q, want error for non-numeric port", st.Status)
	}
}
