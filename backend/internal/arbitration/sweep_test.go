package arbitration

import (
	"os"
	"testing"
	"time"
)

// TestSweep prints how rule settings do on the three homelab captures: moves
// by the still tag at the boundary, walk arrivals caught and their worst lag,
// and moves by the stationary tags. It is how the defaults were chosen; rerun
// it with new captures before changing them. Run with ARBITRATION_SWEEP=1.
func TestSweep(t *testing.T) {
	if os.Getenv("ARBITRATION_SWEEP") == "" {
		t.Skip("set ARBITRATION_SWEEP=1")
	}
	boundary := loadFixture(t, "homelab_boundary_57m.csv.gz")
	walk := loadFixture(t, "homelab_walk_4m.csv.gz")
	stat := loadFixture(t, "homelab_stationary_30m.csv.gz")
	arrivals := []switchAt{{100_000, 2}, {113_000, 3}, {127_000, 1}, {141_000, 2}, {152_000, 3}, {167_000, 1}}
	for _, margin := range []int{6, 8} {
		for _, dwell := range []time.Duration{3 * time.Second, 5 * time.Second, 8 * time.Second} {
			for _, rw := range []time.Duration{0, 5 * time.Minute, 15 * time.Minute} {
				for _, rd := range []time.Duration{30 * time.Second, 60 * time.Second} {
					cfg := DefaultConfig()
					cfg.MarginDB, cfg.Dwell, cfg.ReturnWindow, cfg.ReturnDwell = margin, dwell, rw, rd
					b := replay(t, boundary, cfg)
					w := replay(t, walk, cfg)
					s := replay(t, stat, cfg)
					statMax := 0
					for _, sw := range s {
						statMax = max(statMax, len(after(sw, 60_000)))
					}
					caught, worst, extra := 0, 0, 0
					for _, id := range []int{1, 2} {
						got := after(w[id], 90_000)
						extra += len(got) - len(arrivals)
						for _, a := range arrivals {
							for _, g := range got {
								if g.loc == a.loc && g.ms >= a.ms-3000 && g.ms <= a.ms+20_000 {
									caught++
									worst = max(worst, g.ms-a.ms)
									break
								}
							}
						}
					}
					t.Logf("margin=%d dwell=%-3s returnWindow=%-5s returnDwell=%-4s | boundary T3=%2d T2=%d T4=%d | walk %2d/12 worst=%5.1fs extra=%+d | stationary=%d",
						margin, dwell, rw, rd, len(after(b[3], 360_000)), len(after(b[2], 60_000)), len(after(b[4], 60_000)),
						caught, float64(worst)/1000, extra, statMax)
				}
			}
		}
	}
}
