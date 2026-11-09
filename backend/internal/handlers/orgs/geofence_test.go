package orgs

import (
	"slices"
	"testing"

	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

func ip(v int) *int       { return &v }
func sp(v string) *string { return &v }

func TestValidateGeofenceDefaults(t *testing.T) {
	cases := []struct {
		name    string
		in      organization.GeofenceDefaults
		wantErr bool
	}{
		{"all nil ok", organization.GeofenceDefaults{}, false},
		{"valid full", organization.GeofenceDefaults{RSSIThreshold: ip(-55), AgeOutSeconds: ip(30), AutoOffSeconds: ip(5), Mode: sp("presence")}, false},
		{"valid egress", organization.GeofenceDefaults{Mode: sp("egress")}, false},
		{"bad mode", organization.GeofenceDefaults{Mode: sp("strobe")}, true},
		{"rssi too low", organization.GeofenceDefaults{RSSIThreshold: ip(-121)}, true},
		{"rssi too high", organization.GeofenceDefaults{RSSIThreshold: ip(1)}, true},
		{"rssi boundary 0 ok", organization.GeofenceDefaults{RSSIThreshold: ip(0)}, false},
		{"age_out zero", organization.GeofenceDefaults{AgeOutSeconds: ip(0)}, true},
		{"age_out one ok", organization.GeofenceDefaults{AgeOutSeconds: ip(1)}, false},
		{"auto_off negative", organization.GeofenceDefaults{AutoOffSeconds: ip(-1)}, true},
		{"auto_off zero ok", organization.GeofenceDefaults{AutoOffSeconds: ip(0)}, false},
		{"auto_off at the 60 s ceiling", organization.GeofenceDefaults{AutoOffSeconds: ip(60)}, false},
		{"auto_off over the 60 s ceiling", organization.GeofenceDefaults{AutoOffSeconds: ip(61)}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateGeofenceDefaults(c.in)
			if c.wantErr != (err != nil) {
				t.Fatalf("wantErr=%v got err=%v", c.wantErr, err)
			}
		})
	}
}

// TestStrandedOutputs pins TRA-1273's org-tier half: a defaults change is refused
// when it leaves an egress output that HAD an off edge without one. A row that
// was already invalid is not stranded by the change, so it does not block an
// unrelated edit.
func TestStrandedOutputs(t *testing.T) {
	dev := func(name string, meta map[string]any) outputdevice.OutputDevice {
		return outputdevice.OutputDevice{Name: name, Metadata: meta}
	}
	devs := []outputdevice.OutputDevice{
		dev("Relies on org", map[string]any{"mode": "egress"}),
		dev("Own auto-off", map[string]any{"mode": "egress", "auto_off_seconds": float64(10)}),
		dev("Presence", map[string]any{"mode": "presence"}),
		dev("Mode unset", map[string]any{}),
		dev("Already latched", map[string]any{"mode": "egress", "auto_off_seconds": float64(0)}),
	}
	with30 := organization.GeofenceDefaults{AutoOffSeconds: ip(30)}

	cases := []struct {
		name     string
		old, new organization.GeofenceDefaults
		want     []string
	}{
		{"clearing the org auto-off strands what relied on it",
			with30, organization.GeofenceDefaults{}, []string{"Relies on org", "Mode unset"}},
		{"zeroing it is the same as clearing it",
			with30, organization.GeofenceDefaults{AutoOffSeconds: ip(0)}, []string{"Relies on org", "Mode unset"}},
		{"an unrelated change strands nothing, even beside an already-invalid row",
			with30, organization.GeofenceDefaults{AutoOffSeconds: ip(30), RSSIThreshold: ip(-50)}, nil},
		{"flipping the org mode to egress strands a mode-unset device",
			organization.GeofenceDefaults{Mode: sp("presence")}, organization.GeofenceDefaults{Mode: sp("egress")}, []string{"Mode unset"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strandedOutputs(c.old, c.new, devs)
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestGeofenceDefaultsView_IncludesSystemDefaults(t *testing.T) {
	v := geofenceDefaultsView(organization.GeofenceDefaults{})
	if v.SystemDefaults.RSSIThreshold != -65 || v.SystemDefaults.AgeOutSeconds != 60 || v.SystemDefaults.Mode != "egress" {
		t.Fatalf("system defaults wrong: %+v", v.SystemDefaults)
	}
}
