package mustering

import (
	"strings"
	"testing"

	"github.com/trakrf/platform/backend/internal/models/muster"
)

func TestValidateFloorPlanShape(t *testing.T) {
	tests := []struct {
		name    string
		fp      muster.FloorPlan
		wantErr string // substring; "" = valid
	}{
		{name: "valid https", fp: muster.FloorPlan{ImageURL: "https://x/y.png"}},
		{name: "valid http", fp: muster.FloorPlan{ImageURL: "http://x/y.png"}},
		{name: "valid data url", fp: muster.FloorPlan{ImageURL: "data:image/png;base64,AAAA"}},
		{name: "valid with pins at bounds", fp: muster.FloorPlan{
			ImageURL: "https://x/y.png",
			Pins: []muster.FloorPlanPin{
				{LocationID: 1, XPct: 0, YPct: 0},
				{LocationID: 2, XPct: 100, YPct: 100},
			},
		}},
		{name: "empty url", fp: muster.FloorPlan{ImageURL: "   "}, wantErr: "required"},
		{name: "bad scheme", fp: muster.FloorPlan{ImageURL: "ftp://x/y.png"}, wantErr: "http(s) or data"},
		{name: "too long", fp: muster.FloorPlan{ImageURL: "https://x/" + strings.Repeat("a", 2048)}, wantErr: "2048"},
		{name: "x out of range", fp: muster.FloorPlan{ImageURL: "https://x/y.png", Pins: []muster.FloorPlanPin{{XPct: 100.1, YPct: 0}}}, wantErr: "between 0 and 100"},
		{name: "y negative", fp: muster.FloorPlan{ImageURL: "https://x/y.png", Pins: []muster.FloorPlanPin{{XPct: 0, YPct: -1}}}, wantErr: "between 0 and 100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFloorPlanShape(tt.fp)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
