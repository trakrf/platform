package mustering

import (
	"encoding/json"
	"sync"

	"github.com/trakrf/platform/backend/internal/models/muster"
)

// clientBuffer bounds per-subscriber queue depth. A browser that can't keep up
// drops frames rather than stalling the engine — every delta is idempotent and a
// client can always re-snapshot via GET /api/v1/mustering/status.
const clientBuffer = 64

// EventType is the SSE `event:` name for a mustering frame.
type EventType string

const (
	EventSnapshot EventType = "snapshot"
	EventPresence EventType = "presence"
	EventEntry    EventType = "entry"
	EventEvent    EventType = "event"
)

// Event is one SSE frame: a named event plus its JSON data payload.
type Event struct {
	Type EventType
	Data []byte
}

// SnapshotPayload seeds a connection: zone headcounts + the active event (or
// nil). PersonsOnSite is the sum of non-muster-point zone counts.
type SnapshotPayload struct {
	Zones         []muster.ZonePresence `json:"zones"`
	PersonsOnSite int                   `json:"persons_on_site"`
	Event         *muster.Event         `json:"event"`
}

// PresencePayload is a zone-headcount delta. Persons (per-person location) is
// populated only while an event is active (break-glass). PersonsOnSite is the
// sum of non-muster-point zone counts — the same total the snapshot carries —
// so the dashboard's "on site" tile stays live between snapshots (BUG 3).
type PresencePayload struct {
	Zones         []muster.ZonePresence   `json:"zones"`
	PersonsOnSite int                     `json:"persons_on_site"`
	Persons       []muster.PersonPresence `json:"persons,omitempty"`
}

// entryPayload carries a single entry transition + refreshed counts.
type entryPayload struct {
	Entry  muster.Entry  `json:"entry"`
	Counts muster.Counts `json:"counts"`
}

// eventPayload wraps a muster event lifecycle change.
type eventPayload struct {
	Event muster.Event `json:"event"`
}

// Broadcaster fans engine deltas out to the per-org SSE subscribers. It holds no
// state beyond the subscriber registry — every payload is computed by the engine.
// Single-replica only (TRA-907).
type Broadcaster struct {
	mu   sync.Mutex
	subs map[int]map[*subscriber]struct{}
}

type subscriber struct {
	ch chan Event
}

// NewBroadcaster builds an empty broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: map[int]map[*subscriber]struct{}{}}
}

// Subscribe registers an SSE connection for an org and returns its event channel
// plus a cancel func (safe to call repeatedly). The channel is never closed —
// the SSE handler exits on its request context and cancel removes the
// registration, after which the buffered channel is GC'd.
func (b *Broadcaster) Subscribe(orgID int) (<-chan Event, func()) {
	s := &subscriber{ch: make(chan Event, clientBuffer)}
	b.mu.Lock()
	if b.subs[orgID] == nil {
		b.subs[orgID] = map[*subscriber]struct{}{}
	}
	b.subs[orgID][s] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			if set := b.subs[orgID]; set != nil {
				delete(set, s)
				if len(set) == 0 {
					delete(b.subs, orgID)
				}
			}
			b.mu.Unlock()
		})
	}
	return s.ch, cancel
}

// send fans one event out to every subscriber of an org, dropping for slow
// clients.
func (b *Broadcaster) send(orgID int, ev Event) {
	b.mu.Lock()
	subs := make([]*subscriber, 0, len(b.subs[orgID]))
	for s := range b.subs[orgID] {
		subs = append(subs, s)
	}
	b.mu.Unlock()
	for _, s := range subs {
		select {
		case s.ch <- ev:
		default: // slow client; drop, client can re-snapshot
		}
	}
}

func (b *Broadcaster) BroadcastSnapshot(orgID int, payload SnapshotPayload) {
	if data, err := json.Marshal(payload); err == nil {
		b.send(orgID, Event{Type: EventSnapshot, Data: data})
	}
}

func (b *Broadcaster) BroadcastPresence(orgID int, payload PresencePayload) {
	if data, err := json.Marshal(payload); err == nil {
		b.send(orgID, Event{Type: EventPresence, Data: data})
	}
}

func (b *Broadcaster) BroadcastEntry(orgID int, entry muster.Entry, counts muster.Counts) {
	if data, err := json.Marshal(entryPayload{Entry: entry, Counts: counts}); err == nil {
		b.send(orgID, Event{Type: EventEntry, Data: data})
	}
}

func (b *Broadcaster) BroadcastEvent(orgID int, ev muster.Event) {
	if data, err := json.Marshal(eventPayload{Event: ev}); err == nil {
		b.send(orgID, Event{Type: EventEvent, Data: data})
	}
}
