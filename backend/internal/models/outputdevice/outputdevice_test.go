package outputdevice

import (
	"encoding/json"
	"testing"
)

func TestAutoOffSeconds(t *testing.T) {
	cases := []struct {
		name     string
		metadata any
		want     int
	}{
		// pgx decodes jsonb into map[string]any with numbers as float64 — the
		// real runtime shape.
		{"float64 from jsonb", map[string]any{"auto_off_seconds": float64(30)}, 30},
		{"json.Number", map[string]any{"auto_off_seconds": json.Number("15")}, 15},
		{"int", map[string]any{"auto_off_seconds": 10}, 10},
		{"absent key", map[string]any{"other": float64(1)}, 0},
		{"zero means no auto-off", map[string]any{"auto_off_seconds": float64(0)}, 0},
		{"negative coerced to zero", map[string]any{"auto_off_seconds": float64(-5)}, 0},
		{"nil metadata", nil, 0},
		{"non-map metadata", "garbage", 0},
		{"non-numeric value", map[string]any{"auto_off_seconds": "30"}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := OutputDevice{Metadata: tc.metadata}
			if got := d.AutoOffSeconds(); got != tc.want {
				t.Errorf("AutoOffSeconds() = %d, want %d", got, tc.want)
			}
		})
	}
}

func dev(meta map[string]any) OutputDevice { return OutputDevice{Metadata: meta} }

func TestMode(t *testing.T) {
	if got := dev(nil).Mode(); got != ModeEgress {
		t.Fatalf("nil metadata should default to egress, got %q", got)
	}
	if got := dev(map[string]any{"mode": "presence"}).Mode(); got != ModePresence {
		t.Fatalf("expected presence, got %q", got)
	}
	if got := dev(map[string]any{"mode": "garbage"}).Mode(); got != ModeEgress {
		t.Fatalf("unknown mode should default to egress, got %q", got)
	}
}

func TestAgeOutSeconds(t *testing.T) {
	if _, ok := dev(nil).AgeOutSeconds(); ok {
		t.Fatal("nil metadata should report no override")
	}
	if _, ok := dev(map[string]any{"age_out_seconds": float64(0)}).AgeOutSeconds(); ok {
		t.Fatal("zero should report no override (fall back to global)")
	}
	v, ok := dev(map[string]any{"age_out_seconds": float64(30)}).AgeOutSeconds()
	if !ok || v != 30 {
		t.Fatalf("expected (30,true), got (%d,%v)", v, ok)
	}
}

func TestRSSIThreshold(t *testing.T) {
	if _, ok := dev(nil).RSSIThreshold(); ok {
		t.Fatal("nil metadata should report no override")
	}
	// RSSI is dBm — negatives are valid and must NOT be rejected.
	v, ok := dev(map[string]any{"rssi_threshold": float64(-55)}).RSSIThreshold()
	if !ok || v != -55 {
		t.Fatalf("expected (-55,true), got (%d,%v)", v, ok)
	}
	if _, ok := dev(map[string]any{"rssi_threshold": "nope"}).RSSIThreshold(); ok {
		t.Fatal("non-numeric should report no override")
	}
}

func TestTypeCS463GPO_Value(t *testing.T) {
	// The constant must match the PG enum value added in migration 000032 and
	// the `oneof` validation tags; a drift here is a silently unfireable device.
	if TypeCS463GPO != "csl_cs463_gpo" {
		t.Errorf("TypeCS463GPO = %q, want %q", TypeCS463GPO, "csl_cs463_gpo")
	}
}

func TestOutputDevice_ScanDeviceIDAndReaderBaseTopicJSON(t *testing.T) {
	scanDeviceID := 42
	d := OutputDevice{
		ID:              1,
		ScanDeviceID:    &scanDeviceID,
		ReaderBaseTopic: "trakrf.id/cs463-212",
	}

	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got, ok := m["scan_device_id"]; !ok || got != float64(42) {
		t.Errorf("scan_device_id = %v (present=%v), want 42", got, ok)
	}
	if _, ok := m["reader_base_topic"]; ok {
		t.Error("reader_base_topic must not appear in JSON (transient, json:\"-\")")
	}

	// Round-trip: scan_device_id decodes back correctly.
	var back OutputDevice
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("Unmarshal into OutputDevice: %v", err)
	}
	if back.ScanDeviceID == nil || *back.ScanDeviceID != 42 {
		t.Errorf("round-tripped ScanDeviceID = %v, want 42", back.ScanDeviceID)
	}
	if back.ReaderBaseTopic != "" {
		t.Errorf("ReaderBaseTopic should not decode from JSON, got %q", back.ReaderBaseTopic)
	}
}
