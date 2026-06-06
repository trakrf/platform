package geofence

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

// presence drives presence-mode outputs (TRA-943/TRA-950). Each output, while a
// member tag is present, holds a single per-output timer set to the output's
// age-out and reset on every qualifying read. The ON edge fires on the 0->1
// transition (first read after the output is idle); the OFF edge fires when the
// timer expires — i.e. no member has been read for age-out — at which point the
// output is driven off and forgotten.
//
// This is the level/presence archetype: RFID emits no "tag left" event, so the
// departure is *defined* by the age-out. Using a per-output timer (rather than a
// periodic sweep) makes age-out the single knob: OFF fires exactly age-out after
// the last read, with no added poll latency.
type presence struct {
	mu      sync.Mutex
	outputs map[string]*presenceOutput // "org:outputID" -> state
	closed  bool

	driver outputDriver
	log    zerolog.Logger

	// afterFunc schedules f to run after d; *time.Timer satisfies the return.
	// Injectable so tests fire timers deterministically instead of sleeping.
	afterFunc func(d time.Duration, f func()) timerHandle
}

// timerHandle is the subset of *time.Timer the presence tracker uses.
type timerHandle interface {
	Stop() bool
}

type presenceOutput struct {
	dev   outputdevice.OutputDevice // snapshot, so the timer can drive OFF later
	timer timerHandle
	// gen increments on every observe; the timer callback captures the gen it was
	// scheduled with and no-ops if a newer read has since rescheduled (guards the
	// timer-fire vs concurrent-observe race).
	gen uint64
}

func newPresence(driver outputDriver, log zerolog.Logger) *presence {
	return &presence{
		outputs: map[string]*presenceOutput{},
		driver:  driver,
		log:     log,
		afterFunc: func(d time.Duration, f func()) timerHandle {
			return time.AfterFunc(d, f)
		},
	}
}

func presenceKey(orgID, outputID int) string {
	return fmt.Sprintf("%d:%d", orgID, outputID)
}

// observe records that a member tag was seen at an output and (re)arms the
// output's age-out timer to ttl. It returns true only on the 0->1 transition
// (the caller fires ON); subsequent reads while present return false.
func (p *presence) observe(orgID int, dev outputdevice.OutputDevice, ttl time.Duration, epc string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	k := presenceKey(orgID, dev.ID)
	po := p.outputs[k]
	fresh := po == nil
	if fresh {
		po = &presenceOutput{}
		p.outputs[k] = po
	}
	po.dev = dev
	po.gen++
	gen := po.gen
	if po.timer != nil {
		po.timer.Stop()
	}
	po.timer = p.afterFunc(ttl, func() { p.expire(k, gen) })
	return fresh
}

// expire drives an output off when its age-out timer fires, unless a newer read
// has rescheduled it (stale gen) or it has already been cleared. The Set happens
// outside the lock (best-effort).
func (p *presence) expire(key string, gen uint64) {
	p.mu.Lock()
	po := p.outputs[key]
	if po == nil || po.gen != gen {
		p.mu.Unlock()
		return
	}
	dev := po.dev
	delete(p.outputs, key)
	p.mu.Unlock()

	if err := p.driver.Set(context.Background(), dev, false, 0); err != nil {
		p.log.Error().Err(err).Int("output_device_id", dev.ID).Msg("presence off failed (best-effort)")
	}
}

// Close stops all outstanding timers and blocks further observes. Idempotent.
func (p *presence) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for _, po := range p.outputs {
		if po.timer != nil {
			po.timer.Stop()
		}
	}
	p.outputs = map[string]*presenceOutput{}
}
