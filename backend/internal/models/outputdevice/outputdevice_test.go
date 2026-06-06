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
