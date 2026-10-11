package geofence

import (
	"fmt"
	"sync"
	"time"
)

// pulseGuard drops an egress fire while the same output is still pulsing from an
// earlier one, so two tags arriving together neither stack nor extend the alarm.
// It is keyed per (org, output), deliberately not per epc: the latch already
// dedups a single tag, and the point here is that a different tag must not
// re-trigger a device that is already sounding.
//
// Like the latch, it takes the time from the caller (server receive time), so
// decisions are deterministic and clock-free. It holds one entry per output
// device ever pulsed, so it needs no sweeper.
type pulseGuard struct {
	mu    sync.Mutex
	until map[string]int64 // "org:outputID" -> unix nanos the current pulse ends
}

func newPulseGuard() *pulseGuard {
	return &pulseGuard{until: make(map[string]int64)}
}

func pulseKey(orgID, outputID int) string {
	return fmt.Sprintf("%d:%d", orgID, outputID)
}

// tryStart reports whether a pulse of length d may start at now and, if so,
// marks the output busy until now+d. The output is busy while now < until, so a
// pulse may start at exactly the previous one's end. A non-positive d (no
// device-side auto-off) is never guarded and leaves no mark.
func (g *pulseGuard) tryStart(orgID, outputID int, now time.Time, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	key := pulseKey(orgID, outputID)
	nowNanos := now.UnixNano()

	g.mu.Lock()
	defer g.mu.Unlock()
	if nowNanos < g.until[key] {
		return false
	}
	g.until[key] = nowNanos + d.Nanoseconds()
	return true
}

// release clears the mark a tryStart(orgID, outputID, startedAt, d) made, so a
// drive that failed does not block the next tag for the whole window. It only
// clears the caller's own mark: if a newer pulse has since taken the output, it
// is left alone.
func (g *pulseGuard) release(orgID, outputID int, startedAt time.Time, d time.Duration) {
	if d <= 0 {
		return
	}
	key := pulseKey(orgID, outputID)

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.until[key] == startedAt.UnixNano()+d.Nanoseconds() {
		delete(g.until, key)
	}
}
