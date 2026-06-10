package orgs

import (
	"testing"

	"github.com/trakrf/platform/backend/internal/models/organization"
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

func TestGeofenceDefaultsView_IncludesSystemDefaults(t *testing.T) {
	v := geofenceDefaultsView(organization.GeofenceDefaults{})
	if v.SystemDefaults.RSSIThreshold != -65 || v.SystemDefaults.AgeOutSeconds != 60 || v.SystemDefaults.Mode != "egress" {
		t.Fatalf("system defaults wrong: %+v", v.SystemDefaults)
	}
}
