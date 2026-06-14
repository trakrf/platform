package antennapower

import "testing"

func TestValidatePowers(t *testing.T) {
	cases := []struct {
		name    string
		in      map[string]float64
		wantErr bool
		wantLen int
	}{
		{"empty (get)", map[string]float64{}, false, 0},
		{"valid", map[string]float64{"1": 22.5, "4": 30}, false, 2},
		{"min edge", map[string]float64{"1": 0}, false, 1},
		{"max edge", map[string]float64{"1": 32}, false, 1},
		{"over max", map[string]float64{"1": 32.5}, true, 0},
		{"negative", map[string]float64{"2": -1}, true, 0},
		{"port 0", map[string]float64{"0": 20}, true, 0},
		{"port 17", map[string]float64{"17": 20}, true, 0},
		{"non-numeric port", map[string]float64{"a": 20}, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, msg := validatePowers(tc.in)
			if tc.wantErr && msg == "" {
				t.Fatalf("want error, got none")
			}
			if !tc.wantErr {
				if msg != "" {
					t.Fatalf("unexpected error: %s", msg)
				}
				if len(out) != tc.wantLen {
					t.Fatalf("len = %d, want %d", len(out), tc.wantLen)
				}
			}
		})
	}
}
