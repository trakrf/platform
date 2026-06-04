package geofence

import (
	"sync"
	"time"
)

// Clock is a minimal time source so the latch sweeper can be driven
// deterministically in tests. Mirrors ratelimit.Clock; kept local to avoid
// coupling the geofence package to the rate limiter.
type Clock interface {
	Now() time.Time
}

// RealClock wraps time.Now.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// FakeClock is a test clock advanced manually via Advance.
type FakeClock struct {
	mu sync.Mutex
	t  time.Time
}

// NewFakeClock returns a FakeClock initialized to start.
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{t: start}
}

func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}
