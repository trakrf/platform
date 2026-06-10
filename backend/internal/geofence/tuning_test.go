package geofence

import (
	"testing"

	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

// ptr(int) *int is defined in engine_test.go (same package). sptr covers strings.
func sptr(s string) *string { return &s }

func TestResolve_CodeDefaultsOnly(t *testing.T) {
	got := Resolve(DefaultConfig(), organization.GeofenceDefaults{}, outputdevice.OutputDevice{})
	if got.RSSIThreshold != -65 || got.AgeOutSeconds != 60 || got.Mode != outputdevice.ModeEgress || got.AutoOffSeconds != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestResolve_OrgOverridesCode(t *testing.T) {
	org := organization.GeofenceDefaults{RSSIThreshold: ptr(-50), AgeOutSeconds: ptr(20), AutoOffSeconds: ptr(7), Mode: sptr("presence")}
	got := Resolve(DefaultConfig(), org, outputdevice.OutputDevice{})
	if got.RSSIThreshold != -50 || got.AgeOutSeconds != 20 || got.AutoOffSeconds != 7 || got.Mode != "presence" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolve_DeviceOverridesOrg(t *testing.T) {
	org := organization.GeofenceDefaults{RSSIThreshold: ptr(-50), AgeOutSeconds: ptr(20), AutoOffSeconds: ptr(7), Mode: sptr("presence")}
	dev := outputdevice.OutputDevice{Metadata: map[string]any{
		"rssi_threshold": float64(-40), "age_out_seconds": float64(15),
		"auto_off_seconds": float64(3), "mode": "egress",
	}}
	got := Resolve(DefaultConfig(), org, dev)
	if got.RSSIThreshold != -40 || got.AgeOutSeconds != 15 || got.AutoOffSeconds != 3 || got.Mode != "egress" {
		t.Fatalf("got %+v", got)
	}
}

func TestResolve_PartialOrgFallsThrough(t *testing.T) {
	org := organization.GeofenceDefaults{AgeOutSeconds: ptr(20)} // only age_out set
	got := Resolve(DefaultConfig(), org, outputdevice.OutputDevice{})
	if got.RSSIThreshold != -65 || got.AgeOutSeconds != 20 || got.Mode != outputdevice.ModeEgress || got.AutoOffSeconds != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestResolve_InvalidOrgModeIgnored(t *testing.T) {
	org := organization.GeofenceDefaults{Mode: sptr("garbage")}
	got := Resolve(DefaultConfig(), org, outputdevice.OutputDevice{})
	if got.Mode != outputdevice.ModeEgress {
		t.Fatalf("invalid org mode must fall back to code default, got %q", got.Mode)
	}
}

func TestResolve_DeviceAutoOffZeroOverridesOrg(t *testing.T) {
	// Device explicitly sets auto_off 0; that is a real value and must win over an
	// org default of 9.
	org := organization.GeofenceDefaults{AutoOffSeconds: ptr(9)}
	dev := outputdevice.OutputDevice{Metadata: map[string]any{"auto_off_seconds": float64(0)}}
	got := Resolve(DefaultConfig(), org, dev)
	if got.AutoOffSeconds != 0 {
		t.Fatalf("device auto_off 0 must override org default, got %d", got.AutoOffSeconds)
	}
}

func TestSystemTuning(t *testing.T) {
	got := SystemTuning(DefaultConfig())
	if got.RSSIThreshold != -65 || got.AgeOutSeconds != 60 || got.Mode != outputdevice.ModeEgress || got.AutoOffSeconds != 0 {
		t.Fatalf("got %+v", got)
	}
}
