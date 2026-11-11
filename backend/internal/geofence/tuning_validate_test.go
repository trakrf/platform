package geofence

import (
	"testing"

	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

func TestMaxAutoOffSeconds_IsTheReaderCeiling(t *testing.T) {
	if MaxAutoOffSeconds != 60 {
		t.Fatalf("MaxAutoOffSeconds = %d, want 60 (readerrpc.GpoMaxOn)", MaxAutoOffSeconds)
	}
}

func TestOutputTuningError(t *testing.T) {
	meta := func(kv ...any) outputdevice.OutputDevice {
		m := map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return outputdevice.OutputDevice{Metadata: m}
	}
	none := organization.GeofenceDefaults{}
	org30 := organization.GeofenceDefaults{AutoOffSeconds: ptr(30)}
	orgPresence := organization.GeofenceDefaults{Mode: sptr(outputdevice.ModePresence)}
	orgEgress := organization.GeofenceDefaults{Mode: sptr(outputdevice.ModeEgress)}

	cases := []struct {
		name    string
		org     organization.GeofenceDefaults
		dev     outputdevice.OutputDevice
		wantErr bool
	}{
		// The bug: egress with nothing anywhere supplying an off edge.
		{"egress, no auto-off anywhere", none, meta("mode", "egress"), true},
		{"no metadata at all resolves to egress with no off edge", none, outputdevice.OutputDevice{}, true},
		{"nil metadata map", none, outputdevice.OutputDevice{Metadata: map[string]any(nil)}, true},
		{"org default supplies the off edge", org30, meta("mode", "egress"), false},
		{"explicit 0 overrides the org default", org30, meta("mode", "egress", "auto_off_seconds", float64(0)), true},
		{"explicit 1", none, meta("mode", "egress", "auto_off_seconds", float64(1)), false},
		{"explicit 60 is the ceiling", none, meta("mode", "egress", "auto_off_seconds", float64(60)), false},
		{"explicit 61 is over the ceiling", none, meta("mode", "egress", "auto_off_seconds", float64(61)), true},
		{"over the ceiling in presence mode too", none, meta("mode", "presence", "auto_off_seconds", float64(600)), true},
		{"presence needs no auto-off", none, meta("mode", "presence"), false},
		// A legacy presence row carrying a meaningless 0 must stay editable.
		{"presence with explicit 0", none, meta("mode", "presence", "auto_off_seconds", float64(0)), false},
		{"org mode presence, device mode unset", orgPresence, meta(), false},
		{"org mode egress, nothing supplies auto-off", orgEgress, meta(), true},
		{"device egress overrides org presence", orgPresence, meta("mode", "egress"), true},
		{"negative is not silently unset", org30, meta("mode", "egress", "auto_off_seconds", float64(-1)), true},
		{"fractional seconds", none, meta("mode", "egress", "auto_off_seconds", 2.5), true},
		{"non-numeric", org30, meta("mode", "egress", "auto_off_seconds", "30"), true},
		{"explicit null is unset", org30, meta("mode", "egress", "auto_off_seconds", nil), false},
		{"unknown mode is rejected, not read as egress", org30, meta("mode", "egres"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := OutputTuningError(tc.org, tc.dev)
			if (got != "") != tc.wantErr {
				t.Fatalf("OutputTuningError = %q, wantErr %v", got, tc.wantErr)
			}
		})
	}
}

func TestOrgDefaultsError(t *testing.T) {
	cases := []struct {
		name    string
		d       organization.GeofenceDefaults
		wantErr bool
	}{
		{"unset", organization.GeofenceDefaults{}, false},
		{"zero", organization.GeofenceDefaults{AutoOffSeconds: ptr(0)}, false},
		{"60", organization.GeofenceDefaults{AutoOffSeconds: ptr(60)}, false},
		{"61", organization.GeofenceDefaults{AutoOffSeconds: ptr(61)}, true},
		{"negative", organization.GeofenceDefaults{AutoOffSeconds: ptr(-1)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := OrgDefaultsError(tc.d)
			if (got != "") != tc.wantErr {
				t.Fatalf("OrgDefaultsError = %q, wantErr %v", got, tc.wantErr)
			}
		})
	}
}
