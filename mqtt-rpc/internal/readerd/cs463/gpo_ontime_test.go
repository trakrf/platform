package cs463

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// timerSpy replaces the adapter's timer factory. It records each requested
// on-time and runs the release after a short real delay, so a test can assert
// a 60 s cap without waiting a minute.
type timerSpy struct {
	mu   sync.Mutex
	asks []time.Duration
}

func (s *timerSpy) install(a *Adapter) {
	a.afterFunc = func(d time.Duration, f func()) *time.Timer {
		s.mu.Lock()
		s.asks = append(s.asks, d)
		s.mu.Unlock()
		return time.AfterFunc(10*time.Millisecond, f)
	}
}

func (s *timerSpy) requested() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.asks...)
}

// Fail toward off (Mike, 2026-11-08): an ON with no pulse no longer latches.
// It gets the daemon's default on-time, 30 s unless configured.
func TestGpoSet_NoPulseUsesThe30sDefault(t *testing.T) {
	f := &fakeOps{}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4})
	spy := &timerSpy{}
	spy.install(a)

	if err := a.GpoSet(context.Background(), 2, true, 0); err != nil {
		t.Fatalf("GpoSet: %v", err)
	}
	if got := spy.requested(); !reflect.DeepEqual(got, []time.Duration{30 * time.Second}) {
		t.Errorf("on-time = %v, want [30s]", got)
	}
	time.Sleep(60 * time.Millisecond)
	want := []gpoCall{{port: 2, on: true}, {port: 2, on: false}}
	if got := f.gpoSnapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %+v, want %+v", got, want)
	}
}

func TestGpoSet_NoPulseUsesTheConfiguredDefault(t *testing.T) {
	a := NewAdapter(&fakeOps{}, AdapterConfig{AntennaCount: 4, DefaultOn: 20 * time.Second})
	spy := &timerSpy{}
	spy.install(a)

	if err := a.GpoSet(context.Background(), 1, true, 0); err != nil {
		t.Fatal(err)
	}
	if got := spy.requested(); !reflect.DeepEqual(got, []time.Duration{20 * time.Second}) {
		t.Errorf("on-time = %v, want [20s]", got)
	}
}

// The hard ceiling (Mike, 2026-11-08: "worst case 1 minute. no more than that
// for sure"): a requested 600 s pulse is cut to 60 s, with a warning.
func TestGpoSet_600sPulseIsCappedAt60s(t *testing.T) {
	var buf syncBuffer
	log := zerolog.New(&buf)
	f := &fakeOps{}
	a := NewAdapter(f, AdapterConfig{AntennaCount: 4, Logger: &log})
	spy := &timerSpy{}
	spy.install(a)

	if err := a.GpoSet(context.Background(), 2, true, 600_000); err != nil {
		t.Fatalf("GpoSet: %v", err)
	}
	if got := spy.requested(); !reflect.DeepEqual(got, []time.Duration{60 * time.Second}) {
		t.Errorf("on-time = %v, want [60s]", got)
	}
	time.Sleep(60 * time.Millisecond)
	want := []gpoCall{{port: 2, on: true}, {port: 2, on: false}}
	if got := f.gpoSnapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %+v, want %+v (the capped pulse must still release)", got, want)
	}
	if !strings.Contains(buf.String(), `"requested_ms":600000`) || !strings.Contains(buf.String(), `"capped_ms":60000`) {
		t.Errorf("no clamp warning with both figures:\n%s", buf.String())
	}
}

// A pulse at or under the cap is honoured exactly, and not warned about.
func TestGpoSet_PulseUnderTheCapIsUntouched(t *testing.T) {
	var buf syncBuffer
	log := zerolog.New(&buf)
	a := NewAdapter(&fakeOps{}, AdapterConfig{AntennaCount: 4, Logger: &log})
	spy := &timerSpy{}
	spy.install(a)

	for _, ms := range []int{5000, 60_000} {
		if err := a.GpoSet(context.Background(), 2, true, ms); err != nil {
			t.Fatal(err)
		}
	}
	want := []time.Duration{5 * time.Second, 60 * time.Second}
	if got := spy.requested(); !reflect.DeepEqual(got, want) {
		t.Errorf("on-times = %v, want %v", got, want)
	}
	if strings.Contains(buf.String(), "capped") {
		t.Errorf("warned about a pulse within the cap:\n%s", buf.String())
	}
}

// A configured default above the cap cannot get past it either.
func TestNewAdapter_DefaultAboveTheCapIsCapped(t *testing.T) {
	a := NewAdapter(&fakeOps{}, AdapterConfig{AntennaCount: 4, DefaultOn: 10 * time.Minute})
	if a.defaultOn != readerrpc.GpoMaxOn {
		t.Errorf("defaultOn = %v, want the %v cap", a.defaultOn, readerrpc.GpoMaxOn)
	}
}

func TestGpoOnTimeConstants(t *testing.T) {
	if readerrpc.GpoDefaultOn != 30*time.Second || readerrpc.GpoMaxOn != 60*time.Second {
		t.Errorf("GpoDefaultOn = %v, GpoMaxOn = %v; want 30s and 60s", readerrpc.GpoDefaultOn, readerrpc.GpoMaxOn)
	}
}
