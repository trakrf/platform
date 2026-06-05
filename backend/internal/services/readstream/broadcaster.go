// Package readstream fans out parsed MQTT reads to per-org SSE subscribers,
// in-process. It is the org-enforcement seam for the Live Reads tab (TRA-924):
// the browser no longer holds broker creds, and a subscriber only ever receives
// its own org's reads.
//
// Single-replica only. Multi-replica fan-out needs a shared pub/sub or sticky
// sessions; that is deferred and aligned to TRA-907 (keep one backend replica
// until then).
package readstream

import (
	"regexp"
	"sync"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// clientBuffer bounds per-subscriber queue depth. A browser that can't keep up
// drops reads rather than stalling ingestion — the feed is a live diagnostic,
// not a durable log.
const clientBuffer = 256

// ReadEvent is the wire shape streamed to the browser. JSON tags match the
// frontend ParsedRead interface so the client needs no field remapping.
type ReadEvent struct {
	EPC               string `json:"epc"`
	ReaderKey         string `json:"readerKey"`
	CapturePointName  string `json:"capturePointName"`
	AntennaPort       int    `json:"antennaPort"`
	RSSI              int    `json:"rssi"`
	ReaderTimestampMs int64  `json:"readerTimestampMs"`
}

type subscriber struct {
	ch chan ReadEvent
}

// Broadcaster is a concurrency-safe per-org pub/sub hub.
type Broadcaster struct {
	mu   sync.Mutex
	subs map[int]map[*subscriber]struct{}
}

// New returns an empty broadcaster ready for use.
func New() *Broadcaster {
	return &Broadcaster{subs: make(map[int]map[*subscriber]struct{})}
}

// Subscribe registers a client for an org's read stream. The returned cancel
// func unsubscribes; it is safe to call more than once.
//
// The channel is deliberately never closed. Closing would race Publish, which
// sends (non-blocking) to subscriber channels outside the lock — a concurrent
// cancel + close could panic the send. The SSE handler instead exits on its
// request context, after which map removal stops further sends and the orphaned
// buffered channel is garbage-collected.
func (b *Broadcaster) Subscribe(orgID int) (<-chan ReadEvent, func()) {
	s := &subscriber{ch: make(chan ReadEvent, clientBuffer)}

	b.mu.Lock()
	if b.subs[orgID] == nil {
		b.subs[orgID] = make(map[*subscriber]struct{})
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

// Publish converts parsed reads to events and non-blocking-sends each to every
// subscriber of orgID. A full client buffer drops the event for that client.
// It implements ingest.ReadPublisher.
func (b *Broadcaster) Publish(orgID int, topic string, rs []scanread.Read) {
	if len(rs) == 0 {
		return
	}

	b.mu.Lock()
	set := b.subs[orgID]
	if len(set) == 0 {
		b.mu.Unlock()
		return
	}
	targets := make([]*subscriber, 0, len(set))
	for s := range set {
		targets = append(targets, s)
	}
	b.mu.Unlock()

	key := readerKeyFromTopic(topic)
	for _, r := range rs {
		ev := ReadEvent{
			EPC:               r.EPC,
			ReaderKey:         key,
			CapturePointName:  r.CapturePointName,
			AntennaPort:       r.AntennaPort,
			RSSI:              r.RSSI,
			ReaderTimestampMs: r.ReaderTimestamp.UnixMilli(),
		}
		for _, s := range targets {
			select {
			case s.ch <- ev:
			default: // slow client; drop rather than block ingestion
			}
		}
	}
}

var topicRe = regexp.MustCompile(`^trakrf\.id/([^/]+)/reads$`)

// readerKeyFromTopic extracts the reader key from a `trakrf.id/{key}/reads`
// topic, mirroring the frontend's readerKeyFromTopic. Non-matching topics fall
// back to the full topic string so the key is never empty.
func readerKeyFromTopic(topic string) string {
	if m := topicRe.FindStringSubmatch(topic); m != nil {
		return m[1]
	}
	return topic
}
