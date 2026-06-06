// Package readstream maintains a tag-presence set per browser session and
// streams it as ItemTest-style "Inventory" deltas over SSE (TRA-936). It taps
// the ingest parsed-read stream pre-membership, so unknown EPCs surface too —
// Live Reads is a coverage diagnostic.
//
// The server (not the browser) owns presence because read count and RSSI
// averages are aggregates that need every read: pushing per-tag state deltas
// makes bandwidth scale with tag population, not raw read rate, and keeps the
// counts correct. ENTER fires on first sight, UPDATE is coalesced on re-sight,
// LEAVE fires on expiry; a snapshot seeds each connection and a periodic
// keyframe self-heals any dropped delta.
//
// Presence is tracked PER SESSION, not per org: each connection gets its own
// store, starting empty at connect, so a read count means "reads since you
// started watching" and two operators tuning the same readers never share
// counts. This is a setup/tuning activity, not steady state, so the per-session
// memory is immaterial.
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

// subscriber is one browser session: its own presence store and read-rate
// accounting, plus the delivery channel.
type subscriber struct {
	ch    chan Event
	orgID int
	store *store

	readTotal int64     // reads folded in since the last rate refresh
	rateSince time.Time // start of the current rate window
	lastRate  float64   // reads/sec at the last keyframe
}

func (s *subscriber) send(ev Event) {
	select {
	case s.ch <- ev:
	default: // slow client; drop, keyframe will re-seed
	}
}

// snapshotEvent builds this session's snapshot (its own presence + footer stats).
func (s *subscriber) snapshotEvent() Event {
	p := snapshotPayload{
		Tags:       s.store.snapshot(s.orgID),
		UniqueTags: s.store.uniqueTags(s.orgID),
		ReadRate:   s.lastRate,
	}
	data, _ := json.Marshal(p)
	return Event{Type: eventSnapshot, Data: data}
}

// refreshRate computes reads/sec since the last refresh and resets the window.
func (s *subscriber) refreshRate(now time.Time) {
	if !s.rateSince.IsZero() {
		if elapsed := now.Sub(s.rateSince).Seconds(); elapsed > 0 {
			s.lastRate = float64(s.readTotal) / elapsed
		}
	}
	s.readTotal = 0
	s.rateSince = now
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

// Tracker fans parsed reads out to per-session presence stores and runs the
// flush/sweep/keyframe loop. It holds no shared presence state — each
// subscriber owns its own.
type Tracker struct {
	cfg TrackerConfig

	mu   sync.Mutex
	subs map[int]map[*subscriber]struct{}

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
	t := &Tracker{
		cfg:  cfg.withDefaults(),
		subs: make(map[int]map[*subscriber]struct{}),
		now:  time.Now,
		stop: make(chan struct{}),
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

// Subscribe registers a session for an org's read stream with its own presence
// store and seeds it with an (empty) snapshot. The returned cancel func
// unsubscribes (safe to call repeatedly); the session's store is dropped with it.
//
// The channel is deliberately never closed: closing would race the fan-out send
// (done outside the lock). The SSE handler exits on its request context instead;
// map removal stops further sends and the buffered channel is GC'd.
func (t *Tracker) Subscribe(orgID int) (<-chan Event, func()) {
	s := &subscriber{
		ch:    make(chan Event, clientBuffer),
		orgID: orgID,
		store: newStore(t.cfg.TTL, t.cfg.Coalesce),
	}

	t.mu.Lock()
	if t.subs[orgID] == nil {
		t.subs[orgID] = make(map[*subscriber]struct{})
	}
	t.subs[orgID][s] = struct{}{}
	seed := s.snapshotEvent()
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
				}
			}
			t.mu.Unlock()
		})
	}
	return s.ch, cancel
}

// delivery pairs a session with the events destined for it.
type delivery struct {
	s   *subscriber
	evs []orgEvent
}

// Publish folds a batch of parsed reads into every watching session's presence
// store and fans out the resulting deltas. Implements ingest.ReadPublisher.
// Sessions are the only place reads are tracked, so reads for an unwatched org
// are dropped (lazy, per session).
func (t *Tracker) Publish(orgID int, topic string, reads []scanread.Read) {
	if len(reads) == 0 {
		return
	}
	key := readerKeyFromTopic(topic)
	now := t.now()

	t.mu.Lock()
	var out []delivery
	for s := range t.subs[orgID] {
		var evs []orgEvent
		for _, r := range reads {
			evs = append(evs, s.store.ingest(orgID, key, r, now)...)
		}
		s.readTotal += int64(len(reads))
		if s.rateSince.IsZero() {
			s.rateSince = now
		}
		if len(evs) > 0 {
			out = append(out, delivery{s, evs})
		}
	}
	t.mu.Unlock()

	deliver(out)
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
			var out []delivery
			for _, set := range t.subs {
				for s := range set {
					if evs := append(s.store.flush(now), s.store.sweep(now)...); len(evs) > 0 {
						out = append(out, delivery{s, evs})
					}
				}
			}
			t.mu.Unlock()
			deliver(out)
		case <-keyframe.C:
			t.broadcastKeyframes()
		}
	}
}

// broadcastKeyframes refreshes each session's read rate and pushes it a fresh
// snapshot of its own presence, self-healing any delta it dropped.
func (t *Tracker) broadcastKeyframes() {
	now := t.now()
	t.mu.Lock()
	type seed struct {
		s  *subscriber
		ev Event
	}
	var seeds []seed
	for _, set := range t.subs {
		for s := range set {
			s.refreshRate(now)
			seeds = append(seeds, seed{s, s.snapshotEvent()})
		}
	}
	t.mu.Unlock()
	for _, sd := range seeds {
		sd.s.send(sd.ev)
	}
}

// deliver marshals each session's deltas and sends them to that session only.
func deliver(out []delivery) {
	for _, d := range out {
		for _, oe := range d.evs {
			if ev, ok := marshalEvent(oe); ok {
				d.s.send(ev)
			}
		}
	}
}

// marshalEvent renders an orgEvent to its wire Event. Snapshot events are built
// per session (they carry footer stats); only deltas pass through here.
func marshalEvent(oe orgEvent) (Event, bool) {
	switch oe.typ {
	case eventUpsert:
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
