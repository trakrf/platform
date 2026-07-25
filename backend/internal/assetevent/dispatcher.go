package assetevent

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

const (
	// defaultQueueSize bounds how far delivery may fall behind detection.
	defaultQueueSize = 1024
	// defaultWorkers is how many deliveries run concurrently.
	defaultWorkers = 4
	// drainTimeout caps how long Stop waits for in-flight deliveries.
	drainTimeout = 5 * time.Second
	// deliverTimeout caps one delivery attempt end to end.
	deliverTimeout = 15 * time.Second
)

// defaultRetryDelays is the Phase 1 retry ladder: one quick retry, one slower
// one, then give up and count the drop.
//
// This deliberately does NOT implement TRA-398's 1s/5s/30s/5m/30m x 5 ladder,
// and cannot: in-process retries die with the pod, so a long ladder is a
// promise the process can't keep. Honoring it requires the durable outbox
// table, which is Phase 2. Until then the contract documented to integrators is
// at-most-once.
var defaultRetryDelays = []time.Duration{time.Second, 5 * time.Second}

// Dispatcher is the bounded queue and worker pool between detection and
// delivery. Enqueue never blocks: ingestion must never be held up by a
// customer's slow endpoint, so a full queue drops the event and counts it.
//
// This is the same call as QoS0 fire-and-forget ingestion. A queue that grows
// or retries forever produces deliveries describing a world that has already
// moved on, which is worse than not delivering at all.
type Dispatcher struct {
	sinks   []Sink
	queue   chan AssetMoved
	workers int
	log     zerolog.Logger

	retryDelays []time.Duration
	// sleep is the retry delay, injectable so tests do not wait in real time.
	sleep func(time.Duration)

	wg       sync.WaitGroup
	stopOnce sync.Once
	started  bool
}

// NewDispatcher builds a dispatcher over the given sinks. Nil sinks are
// tolerated and skipped, so a disabled transport can be wired unconditionally.
func NewDispatcher(sinks []Sink, log *zerolog.Logger) *Dispatcher {
	l := log.With().Str("component", "assetevent").Logger()
	return &Dispatcher{
		sinks:       sinks,
		queue:       make(chan AssetMoved, defaultQueueSize),
		workers:     defaultWorkers,
		log:         l,
		retryDelays: defaultRetryDelays,
		sleep:       time.Sleep,
	}
}

// Start launches the worker pool. Idempotent in the sense that calling it twice
// is a programming error the second call ignores.
func (d *Dispatcher) Start() {
	if d == nil || d.started {
		return
	}
	d.started = true
	for i := 0; i < d.workers; i++ {
		d.wg.Add(1)
		go d.work()
	}
	d.log.Info().Int("workers", d.workers).Int("queue_size", cap(d.queue)).Msg("asset event dispatcher started")
}

// Stop closes the queue and waits for in-flight deliveries to finish, up to
// drainTimeout. Events still queued when the timeout expires are lost — the
// at-most-once contract, stated plainly.
func (d *Dispatcher) Stop() {
	if d == nil {
		return
	}
	d.stopOnce.Do(func() {
		close(d.queue)
		done := make(chan struct{})
		go func() {
			d.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(drainTimeout):
			remaining := len(d.queue)
			if remaining > 0 {
				metricDropped.WithLabelValues("shutdown").Add(float64(remaining))
			}
			d.log.Warn().Int("undelivered", remaining).Msg("asset event dispatcher drain timed out")
		}
	})
}

// Enqueue hands an event to the workers. Non-blocking by construction: when the
// queue is full the event is dropped and counted rather than applying
// backpressure to the scan path.
func (d *Dispatcher) Enqueue(ev AssetMoved) {
	if d == nil {
		return
	}
	select {
	case d.queue <- ev:
	default:
		metricDropped.WithLabelValues("queue_full").Inc()
		d.log.Warn().
			Int("org_id", ev.OrgID).
			Int("asset_id", ev.Asset.ID).
			Str("delivery_id", ev.DeliveryID).
			Msg("asset event dropped: delivery queue full")
	}
}

func (d *Dispatcher) work() {
	defer d.wg.Done()
	for ev := range d.queue {
		d.deliver(ev)
	}
}

// deliver runs one event through every sink. Each sink gets its own retry
// budget, and a failing sink never prevents another from being tried.
func (d *Dispatcher) deliver(ev AssetMoved) {
	for _, s := range d.sinks {
		if s == nil {
			continue
		}
		d.deliverTo(s, ev)
	}
}

func (d *Dispatcher) deliverTo(s Sink, ev AssetMoved) {
	for attempt := 0; ; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), deliverTimeout)
		metricDeliveryAttempts.Inc()
		err := s.Deliver(ctx, ev)
		cancel()
		if err == nil {
			return
		}
		metricDeliveryErrors.Inc()

		if attempt >= len(d.retryDelays) {
			metricDropped.WithLabelValues("retries_exhausted").Inc()
			d.log.Error().Err(err).
				Int("org_id", ev.OrgID).
				Int("asset_id", ev.Asset.ID).
				Str("delivery_id", ev.DeliveryID).
				Int("attempts", attempt+1).
				Msg("asset event delivery failed; giving up (at-most-once)")
			return
		}

		delay := jitter(d.retryDelays[attempt])
		d.log.Warn().Err(err).
			Str("delivery_id", ev.DeliveryID).
			Dur("retry_in", delay).
			Msg("asset event delivery failed; retrying")
		d.sleep(delay)
	}
}

// jitter spreads retries by +/-20% so a fleet of readers hitting one dead
// endpoint doesn't retry in lockstep.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	spread := float64(d) * 0.2
	return time.Duration(float64(d) - spread + rand.Float64()*2*spread) //nolint:gosec // jitter, not crypto
}
