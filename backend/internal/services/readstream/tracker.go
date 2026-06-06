// Package readstream maintains a server-authoritative tag-presence set per org
// and streams it to the browser as ItemTest-style "Inventory" deltas over SSE
// (TRA-936). It taps the ingest parsed-read stream pre-membership, so unknown
// EPCs surface too — Live Reads is a coverage diagnostic.
//
// The server (not the browser) owns presence because read count and RSSI
// averages are aggregates that need every read: pushing per-tag state deltas
// makes bandwidth scale with tag population, not raw read rate, and keeps the
// counts correct. ENTER fires on first sight, UPDATE is coalesced on re-sight,
// LEAVE fires on expiry; a snapshot seeds each connection and a periodic
// keyframe self-heals any dropped delta.
//
// Single-replica only. Multi-replica fan-out needs shared pub/sub or sticky
// sessions; deferred and aligned to TRA-907 (keep one backend replica).
package readstream

import (
	"encoding/json"
	"regexp"
	"sync"
	"time"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// clientBuffer bounds per-subscriber queue depth. A browser that can't keep up
// drops events rather than stalling ingestion — coalesced state is idempotent
// and the periodic keyframe re-seeds it, so a dropped delta self-heals.
const clientBuffer = 256

// Default presence tuning (the KEYPR 30s window; sweep/coalesce well under it).
const (
	defaultTTL              = 30 * time.Second
	defaultCoalesce         = 1 * time.Second
	defaultTickInterval     = 1 * time.Second
	defaultKeyframeInterval = 30 * time.Second
)

// Event is one SSE frame: a named event plus its JSON data payload.
type Event struct {
	Type EventType
	Data []byte
}

// snapshotPayload seeds/reconciles client state and carries the footer stats
// (ItemTest's Unique Tags + Read Rate).
type snapshotPayload struct {
	Tags       []TagState `json:"tags"`
	UniqueTags int        `json:"uniqueTags"`
	ReadRate   float64    `json:"readRate"`
}

// leavePayload identifies an evicted tag.
type leavePayload struct {
	ReaderKey string `json:"readerKey"`
	EPC       string `json:"epc"`
}

type subscriber struct {
	ch chan Event
}

// TrackerConfig tunes the presence windows and background cadences.
type TrackerConfig struct {
	TTL              time.Duration // sliding presence window
	Coalesce         time.Duration // min interval between UPDATEs per tag
	TickInterval     time.Duration // flush + sweep cadence (must be << TTL)
	KeyframeInterval time.Duration // full-snapshot self-heal cadence
}

func (c TrackerConfig) withDefaults() TrackerConfig {
	if c.TTL <= 0 {
		c.TTL = defaultTTL
	}
	if c.Coalesce <= 0 {
		c.Coalesce = defaultCoalesce
	}
	if c.TickInterval <= 0 {
		c.TickInterval = defaultTickInterval
	}
	if c.KeyframeInterval <= 0 {
		c.KeyframeInterval = defaultKeyframeInterval
	}
	return c
}

// Tracker is the concurrency-safe presence hub: it owns the pure store, fans
// presence deltas out to per-org SSE subscribers, and runs the flush/sweep/
// keyframe loop.
type Tracker struct {
	cfg TrackerConfig

	mu    sync.Mutex
	store *store
	subs  map[int]map[*subscriber]struct{}

	// read-rate accounting (footer stat); reads since the last keyframe per org.
	readTotals map[int]int64
	rateSince  map[int]time.Time
	lastRate   map[int]float64

	now      func() time.Time
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// New builds a Tracker with production defaults and starts its background loop.
// It satisfies ingest.ReadPublisher. Call Stop on shutdown.
func New() *Tracker { return NewTracker(TrackerConfig{}) }

// NewTracker builds a Tracker with the given config (zero fields take defaults)
// and starts its background loop.
func NewTracker(cfg TrackerConfig) *Tracker {
	cfg = cfg.withDefaults()
	t := &Tracker{
		cfg:        cfg,
		store:      newStore(cfg.TTL, cfg.Coalesce),
		subs:       make(map[int]map[*subscriber]struct{}),
		readTotals: make(map[int]int64),
		rateSince:  make(map[int]time.Time),
		lastRate:   make(map[int]float64),
		now:        time.Now,
		stop:       make(chan struct{}),
	}
	t.wg.Add(1)
	go t.run()
	return t
}

// Stop halts the background loop. Idempotent.
func (t *Tracker) Stop() {
	t.stopOnce.Do(func() { close(t.stop) })
	t.wg.Wait()
}

// Subscribe registers a client for an org's presence stream and seeds it with a
// snapshot. The returned cancel func unsubscribes (safe to call repeatedly).
//
// The channel is deliberately never closed: closing would race the fan-out send
// (done outside the lock). The SSE handler exits on its request context instead;
// map removal stops further sends and the buffered channel is GC'd.
func (t *Tracker) Subscribe(orgID int) (<-chan Event, func()) {
	s := &subscriber{ch: make(chan Event, clientBuffer)}

	t.mu.Lock()
	if t.subs[orgID] == nil {
		t.subs[orgID] = make(map[*subscriber]struct{})
	}
	t.subs[orgID][s] = struct{}{}
	seed := t.snapshotEventLocked(orgID)
	t.mu.Unlock()

	// Non-blocking: a fresh buffered channel always has room for the seed.
	s.ch <- seed

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			t.mu.Lock()
			if set := t.subs[orgID]; set != nil {
				delete(set, s)
				if len(set) == 0 {
					delete(t.subs, orgID)
					// Last watcher gone: discard presence + rate state so the next
					// session's counts start from zero (lazy tracking).
					t.store.reset(orgID)
					delete(t.readTotals, orgID)
					delete(t.rateSince, orgID)
					delete(t.lastRate, orgID)
				}
			}
			t.mu.Unlock()
		})
	}
	return s.ch, cancel
}

// Publish folds a batch of parsed reads into the presence set and fans out any
// resulting ENTER deltas immediately. Implements ingest.ReadPublisher.
func (t *Tracker) Publish(orgID int, topic string, reads []scanread.Read) {
	if len(reads) == 0 {
		return
	}
	key := readerKeyFromTopic(topic)
	now := t.now()

	t.mu.Lock()
	// Lazy tracking: only accumulate while someone is watching this org, so read
	// counts mean "reads since you started watching" (and idle orgs cost nothing).
	if len(t.subs[orgID]) == 0 {
		t.mu.Unlock()
		return
	}
	var evs []orgEvent
	for _, r := range reads {
		evs = append(evs, t.store.ingest(orgID, key, r, now)...)
	}
	t.readTotals[orgID] += int64(len(reads))
	if _, ok := t.rateSince[orgID]; !ok {
		t.rateSince[orgID] = now
	}
	t.mu.Unlock()

	t.fanout(evs)
}

// run drives the flush/sweep tick and the keyframe broadcast.
func (t *Tracker) run() {
	defer t.wg.Done()
	tick := time.NewTicker(t.cfg.TickInterval)
	defer tick.Stop()
	keyframe := time.NewTicker(t.cfg.KeyframeInterval)
	defer keyframe.Stop()

	for {
		select {
		case <-t.stop:
			return
		case <-tick.C:
			now := t.now()
			t.mu.Lock()
			evs := append(t.store.flush(now), t.store.sweep(now)...)
			t.mu.Unlock()
			t.fanout(evs)
		case <-keyframe.C:
			t.broadcastKeyframes()
		}
	}
}

// broadcastKeyframes recomputes per-org read rate and pushes a fresh snapshot to
// every subscriber, self-healing any delta dropped by a slow client.
func (t *Tracker) broadcastKeyframes() {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	for orgID, set := range t.subs {
		t.refreshRateLocked(orgID, now)
		seed := t.snapshotEventLocked(orgID)
		for s := range set {
			select {
			case s.ch <- seed:
			default:
			}
		}
	}
}

// refreshRateLocked computes reads/sec for an org since the last refresh and
// resets the window. Caller holds the lock.
func (t *Tracker) refreshRateLocked(orgID int, now time.Time) {
	since, ok := t.rateSince[orgID]
	if ok {
		if elapsed := now.Sub(since).Seconds(); elapsed > 0 {
			t.lastRate[orgID] = float64(t.readTotals[orgID]) / elapsed
		}
	}
	t.readTotals[orgID] = 0
	t.rateSince[orgID] = now
}

// snapshotEventLocked builds a snapshot Event for an org. Caller holds the lock.
func (t *Tracker) snapshotEventLocked(orgID int) Event {
	p := snapshotPayload{
		Tags:       t.store.snapshot(orgID),
		UniqueTags: t.store.uniqueTags(orgID),
		ReadRate:   t.lastRate[orgID],
	}
	data, _ := json.Marshal(p)
	return Event{Type: eventSnapshot, Data: data}
}

// fanout marshals presence deltas and non-blocking-sends each to its org's
// subscribers. A full client buffer drops the event (self-heals on keyframe).
func (t *Tracker) fanout(evs []orgEvent) {
	for _, oe := range evs {
		ev, ok := marshalEvent(oe)
		if !ok {
			continue
		}
		t.mu.Lock()
		set := t.subs[oe.orgID]
		targets := make([]*subscriber, 0, len(set))
		for s := range set {
			targets = append(targets, s)
		}
		t.mu.Unlock()

		for _, s := range targets {
			select {
			case s.ch <- ev:
			default: // slow client; drop, keyframe will re-seed
			}
		}
	}
}

// marshalEvent renders an orgEvent to its wire Event. Snapshot events are built
// elsewhere (they carry footer stats); only deltas pass through here.
func marshalEvent(oe orgEvent) (Event, bool) {
	switch oe.typ {
	case eventEnter, eventUpdate:
		data, err := json.Marshal(oe.tag)
		if err != nil {
			return Event{}, false
		}
		return Event{Type: oe.typ, Data: data}, true
	case eventLeave:
		data, err := json.Marshal(leavePayload{ReaderKey: oe.tag.ReaderKey, EPC: oe.tag.EPC})
		if err != nil {
			return Event{}, false
		}
		return Event{Type: oe.typ, Data: data}, true
	default:
		return Event{}, false
	}
}

var topicRe = regexp.MustCompile(`^trakrf\.id/([^/]+)/reads$`)

// readerKeyFromTopic extracts the reader key from a `trakrf.id/{key}/reads`
// topic, mirroring the frontend. Non-matching topics fall back to the full topic
// string so the key is never empty.
func readerKeyFromTopic(topic string) string {
	if m := topicRe.FindStringSubmatch(topic); m != nil {
		return m[1]
	}
	return topic
}
