package readerconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/readerrpc"
)

func ptr(s string) *string { return &s }

func TestBaseTopicForDevice(t *testing.T) {
	cases := []struct {
		name string
		dev  *scandevice.ScanDevice
		want string
	}{
		{"strips trailing /reads", &scandevice.ScanDevice{PublishTopic: ptr("trakrf.id/cs463-212/reads")}, "trakrf.id/cs463-212"},
		{"no /reads suffix kept verbatim", &scandevice.ScanDevice{PublishTopic: ptr("trakrf.id/cs463-212")}, "trakrf.id/cs463-212"},
		{"only strips a trailing segment", &scandevice.ScanDevice{PublishTopic: ptr("trakrf.id/reads-room/reads")}, "trakrf.id/reads-room"},
		{"nil publish_topic -> empty", &scandevice.ScanDevice{PublishTopic: nil}, ""},
		{"empty publish_topic -> empty", &scandevice.ScanDevice{PublishTopic: ptr("")}, ""},
		{"nil device -> empty", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := baseTopicForDevice(tc.dev); got != tc.want {
				t.Fatalf("baseTopicForDevice = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateTxPower(t *testing.T) {
	ac := func(p float64, en bool) readerrpc.AntennaConfig {
		return readerrpc.AntennaConfig{Antenna: 1, Enabled: en, PowerDBm: p}
	}
	cases := []struct {
		name    string
		cfg     readerrpc.ReaderConfig
		wantErr bool
	}{
		{"empty ok", readerrpc.ReaderConfig{}, false},
		{"in range", readerrpc.ReaderConfig{Antennas: []readerrpc.AntennaConfig{ac(22.5, true)}}, false},
		{"min edge", readerrpc.ReaderConfig{Antennas: []readerrpc.AntennaConfig{ac(10, true)}}, false},
		{"max edge", readerrpc.ReaderConfig{Antennas: []readerrpc.AntennaConfig{ac(31.5, true)}}, false},
		{"under min (enabled)", readerrpc.ReaderConfig{Antennas: []readerrpc.AntennaConfig{ac(5, true)}}, true},
		{"negative (enabled)", readerrpc.ReaderConfig{Antennas: []readerrpc.AntennaConfig{{Antenna: 2, Enabled: true, PowerDBm: -1}}}, true},
		{"over max (enabled)", readerrpc.ReaderConfig{Antennas: []readerrpc.AntennaConfig{ac(32, true)}}, true},
		{"out of range but disabled is ok", readerrpc.ReaderConfig{Antennas: []readerrpc.AntennaConfig{ac(0, false)}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := validateTxPower(tc.cfg)
			if tc.wantErr && msg == "" {
				t.Fatalf("want error, got none")
			}
			if !tc.wantErr && msg != "" {
				t.Fatalf("unexpected error: %s", msg)
			}
		})
	}
}

// fakeRPC is a stand-in RPCClient used to confirm the interface is satisfiable
// by a fake (so serve can inject a nil interface) and to exercise typed wrappers.
//
// Per-method error fields (capsErr, getErr, setErr) take precedence over the
// shared err field when non-nil, so GetCapabilities can succeed while
// GetOperProfile/SetOperProfile return a BusyError (busy→409 coverage).
// Existing tests that only set err continue to work unchanged.
type fakeRPC struct {
	caps    readerrpc.Capabilities
	cfg     readerrpc.ReaderConfig
	setRes  readerrpc.SetConfigResult
	lastSet readerrpc.ReaderConfig
	// shared fallback error — used by any method whose per-method field is nil
	err error
	// per-method overrides; non-nil takes precedence over err
	capsErr error
	getErr  error
	setErr  error
}

func (f *fakeRPC) GetCapabilities(_ context.Context, _ string) (readerrpc.Capabilities, error) {
	if f.capsErr != nil {
		return f.caps, f.capsErr
	}
	return f.caps, f.err
}
func (f *fakeRPC) GetOperProfile(_ context.Context, _ string, _ bool) (readerrpc.ReaderConfig, error) {
	if f.getErr != nil {
		return f.cfg, f.getErr
	}
	return f.cfg, f.err
}
func (f *fakeRPC) SetOperProfile(_ context.Context, _ string, cfg readerrpc.ReaderConfig, _ bool) (readerrpc.SetConfigResult, error) {
	f.lastSet = cfg
	if f.setErr != nil {
		return f.setRes, f.setErr
	}
	return f.setRes, f.err
}

// Compile-time assertion that the fake satisfies the seam.
var _ RPCClient = (*fakeRPC)(nil)

func TestNewHandler_NilRPCIsAllowed(t *testing.T) {
	// A nil interface must be storable so serve can disable reader control.
	h := NewHandler(nil, nil)
	if h.rpc != nil {
		t.Fatalf("expected nil rpc")
	}
}

func TestEnabledPorts(t *testing.T) {
	got := enabledPorts(readerrpc.ReaderConfig{Antennas: []readerrpc.AntennaConfig{
		{Antenna: 1, Enabled: true}, {Antenna: 2, Enabled: false}, {Antenna: 3, Enabled: true},
	}})
	if !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("enabledPorts = %v, want [1 3]", got)
	}
}

func TestWantForce(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/x?force=true", nil)
	if !wantForce(r) {
		t.Fatal("force=true not detected")
	}
	r2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	if wantForce(r2) {
		t.Fatal("force should default false")
	}
}
