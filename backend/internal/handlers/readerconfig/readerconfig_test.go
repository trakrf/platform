package readerconfig

import (
	"context"
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
	cases := []struct {
		name    string
		cfg     readerrpc.ReaderConfig
		wantErr bool
	}{
		{"empty config ok", readerrpc.ReaderConfig{}, false},
		{"in range", readerrpc.ReaderConfig{TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 22.5}}}, false},
		{"min edge", readerrpc.ReaderConfig{TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 0}}}, false},
		{"max edge", readerrpc.ReaderConfig{TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 32}}}, false},
		{"over max", readerrpc.ReaderConfig{TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 32.5}}}, true},
		{"negative", readerrpc.ReaderConfig{TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 2, Power: -1}}}, true},
		{"one bad among many", readerrpc.ReaderConfig{TxPowerDBm: []readerrpc.AntennaPower{{Antenna: 1, Power: 20}, {Antenna: 2, Power: 99}}}, true},
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
type fakeRPC struct {
	caps    readerrpc.Capabilities
	cfg     readerrpc.ReaderConfig
	setRes  readerrpc.SetConfigResult
	lastSet readerrpc.ReaderConfig
	err     error
}

func (f *fakeRPC) GetCapabilities(_ context.Context, _ string) (readerrpc.Capabilities, error) {
	return f.caps, f.err
}
func (f *fakeRPC) GetConfig(_ context.Context, _ string) (readerrpc.ReaderConfig, error) {
	return f.cfg, f.err
}
func (f *fakeRPC) SetConfig(_ context.Context, _ string, cfg readerrpc.ReaderConfig) (readerrpc.SetConfigResult, error) {
	f.lastSet = cfg
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
