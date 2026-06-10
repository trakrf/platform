package organization

import "testing"

func TestParseGeofenceDefaults_AllFields(t *testing.T) {
	md := map[string]any{"geofence_defaults": map[string]any{
		"rssi_threshold": float64(-55), "age_out_seconds": float64(30),
		"auto_off_seconds": float64(5), "mode": "presence",
	}}
	d := ParseGeofenceDefaults(md)
	if d.RSSIThreshold == nil || *d.RSSIThreshold != -55 {
		t.Fatalf("rssi: %v", d.RSSIThreshold)
	}
	if d.AgeOutSeconds == nil || *d.AgeOutSeconds != 30 {
		t.Fatalf("age_out: %v", d.AgeOutSeconds)
	}
	if d.AutoOffSeconds == nil || *d.AutoOffSeconds != 5 {
		t.Fatalf("auto_off: %v", d.AutoOffSeconds)
	}
	if d.Mode == nil || *d.Mode != "presence" {
		t.Fatalf("mode: %v", d.Mode)
	}
}

func TestParseGeofenceDefaults_Absent(t *testing.T) {
	d := ParseGeofenceDefaults(map[string]any{})
	if d.RSSIThreshold != nil || d.AgeOutSeconds != nil || d.AutoOffSeconds != nil || d.Mode != nil {
		t.Fatalf("expected all nil, got %+v", d)
	}
}

func TestParseGeofenceDefaults_PartialAndBlankMode(t *testing.T) {
	md := map[string]any{"geofence_defaults": map[string]any{
		"age_out_seconds": float64(20), "mode": "",
	}}
	d := ParseGeofenceDefaults(md)
	if d.AgeOutSeconds == nil || *d.AgeOutSeconds != 20 {
		t.Fatalf("age_out: %v", d.AgeOutSeconds)
	}
	if d.RSSIThreshold != nil || d.AutoOffSeconds != nil {
		t.Fatalf("expected unset rssi/auto_off, got %+v", d)
	}
	if d.Mode != nil {
		t.Fatalf("blank mode must be treated as unset, got %v", *d.Mode)
	}
}
