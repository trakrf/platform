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
