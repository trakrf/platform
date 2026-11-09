package geofence

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
)

// driveFunc drives one output and records the drive; Engine.drive satisfies
// it. It reports whether the drive succeeded.
type driveFunc func(ctx context.Context, orgID int, dev outputdevice.OutputDevice, on bool, offAfter int, at time.Time, cause storage.DriveCause, alarmID *int64) bool

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

	drive driveFunc
	log   zerolog.Logger

	// afterFunc schedules f to run after d; *time.Timer satisfies the return.
	// Injectable so tests fire timers deterministically instead of sleeping.
	afterFunc func(d time.Duration, f func()) timerHandle
}

// timerHandle is the subset of *time.Timer the presence tracker uses.
type timerHandle interface {
	Stop() bool
}

type presenceOutput struct {
	orgID int                       // so the age-out OFF is recorded under its org
	dev   outputdevice.OutputDevice // snapshot, so the timer can drive OFF later
	timer timerHandle
	// gen increments on every observe; the timer callback captures the gen it was
	// scheduled with and no-ops if a newer read has since rescheduled (guards the
	// timer-fire vs concurrent-observe race).
	gen uint64
	// wanted is set once the engine has tried to drive this output on (not for
	// a startup-grace baseline); onAt is the read time of the last ON that
	// succeeded. Together they decide when a refresh is due (refreshDue).
	wanted bool
	onAt   time.Time
}

// presenceRefreshEvery is how often a held presence output's ON is re-sent
// (TRA-1138). Every ON carries the reader's on-time (readerrpc.GpoDefaultOn,
// 30 s), and the device turns itself off when it runs out. So a wanted alarm
// holds while reads keep arriving, survives two lost refreshes in a row, and a
// lost connection stops it within the on-time. Refreshes piggyback on incoming
// reads: no reads means no refresh, and the age-out OFF follows anyway.
const presenceRefreshEvery = 10 * time.Second

// driven records the result of an ON the engine sent for an output: on the
// edge, or on a refresh. A failed ON leaves onAt alone, so the next read
// retries it.
func (p *presence) driven(orgID, outputID int, at time.Time, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	po := p.outputs[presenceKey(orgID, outputID)]
	if po == nil {
		return // aged out meanwhile; the OFF already went
	}
	po.wanted = true
	if ok {
		po.onAt = at
	}
}

// refreshDue reports whether a held output's ON should be re-sent for a read at
// at: it is wanted, and its last good ON is presenceRefreshEvery old (or never
// succeeded). A startup-grace baseline is never wanted, so never refreshed.
func (p *presence) refreshDue(orgID, outputID int, at time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	po := p.outputs[presenceKey(orgID, outputID)]
	return po != nil && po.wanted && at.Sub(po.onAt) >= presenceRefreshEvery
}

func newPresence(drive driveFunc, log zerolog.Logger) *presence {
	return &presence{
		outputs: map[string]*presenceOutput{},
		drive:   drive,
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
	po.orgID = orgID
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
// has rescheduled it (stale gen) or it has already been cleared. The drive
// happens outside the lock (best-effort) and is recorded with cause age_out.
func (p *presence) expire(key string, gen uint64) {
	p.mu.Lock()
	po := p.outputs[key]
	if po == nil || po.gen != gen {
		p.mu.Unlock()
		return
	}
	orgID, dev := po.orgID, po.dev
	delete(p.outputs, key)
	p.mu.Unlock()

	// drive logs and metrics a failure itself.
	p.drive(context.Background(), orgID, dev, false, 0, time.Now(), storage.DriveCauseAgeOut, nil)
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
