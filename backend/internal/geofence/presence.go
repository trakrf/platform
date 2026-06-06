package geofence

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

// presence tracks, per (org, output device), the set of member EPCs currently
// "here" and their last-seen time. observe() reports the 0->1 edge (fire ON);
// the sweeper detects the 1->0 edge (last member aged out) and drives the output
// OFF. This is the level/presence archetype: RFID emits no "tag left" event, so
// departure is *defined* by the per-output age-out window.
type presence struct {
	mu      sync.Mutex
	outputs map[string]*presenceOutput // "org:outputID" -> state

	driver   outputDriver
	clk      Clock
	log      zerolog.Logger
	sweepInt time.Duration

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

type presenceOutput struct {
	dev     outputdevice.OutputDevice // snapshot, so the sweeper can drive OFF
	ttl     time.Duration
	members map[string]int64 // epc -> last-seen unix nanos
}

func newPresence(driver outputDriver, sweepInterval time.Duration, clk Clock, log zerolog.Logger) *presence {
	if clk == nil {
		clk = RealClock{}
	}
	p := &presence{
		outputs:  map[string]*presenceOutput{},
		driver:   driver,
		clk:      clk,
		log:      log,
		sweepInt: sweepInterval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go p.sweepLoop()
	return p
}

func presenceKey(orgID, outputID int) string {
	return fmt.Sprintf("%d:%d", orgID, outputID)
}

// observe records that a member EPC was seen at an output, refreshing the output
// snapshot + ttl. It returns true only on the 0->1 transition (the caller fires
// ON); subsequent reads while present return false.
func (p *presence) observe(orgID int, dev outputdevice.OutputDevice, ttl time.Duration, epc string, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	k := presenceKey(orgID, dev.ID)
	po := p.outputs[k]
	if po == nil {
		po = &presenceOutput{members: map[string]int64{}}
		p.outputs[k] = po
	}
	po.dev = dev
	po.ttl = ttl
	was := len(po.members) > 0
	po.members[epc] = now.UnixNano()
	return !was
}

func (p *presence) sweepLoop() {
	defer close(p.done)
	if p.sweepInt <= 0 {
		<-p.stop
		return
	}
	t := time.NewTicker(p.sweepInt)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.sweep(context.Background())
		}
	}
}

// sweep ages out members past each output's ttl and drives OFF any output whose
// last member just left. Set calls happen outside the lock (best-effort).
func (p *presence) sweep(ctx context.Context) {
	p.mu.Lock()
	now := p.clk.Now().UnixNano()
	var cleared []outputdevice.OutputDevice
	for k, po := range p.outputs {
		was := len(po.members) > 0
		for epc, ls := range po.members {
			if ls < now-po.ttl.Nanoseconds() {
				delete(po.members, epc)
			}
		}
		if was && len(po.members) == 0 {
			cleared = append(cleared, po.dev)
			delete(p.outputs, k)
		}
	}
	p.mu.Unlock()

	for _, d := range cleared {
		if err := p.driver.Set(ctx, d, false, 0); err != nil {
			p.log.Error().Err(err).Int("output_device_id", d.ID).Msg("presence off failed (best-effort)")
		}
	}
}

// Close stops the sweeper. Safe to call multiple times.
func (p *presence) Close() {
	p.closeOnce.Do(func() {
		close(p.stop)
		<-p.done
	})
}
