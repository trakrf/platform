package readstream

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// EventType is the SSE `event:` name for a presence delta.
type EventType string

const (
	eventSnapshot EventType = "snapshot"
	eventEnter    EventType = "enter"
	eventUpdate   EventType = "update"
	eventLeave    EventType = "leave"
)

// TagState is the per-(reader,epc) presence record streamed to the browser. It
// is the ItemTest "Inventory" row: read count + RSSI aggregates + first/last
// seen, all computed server-side from the parsed read stream (the server is the
// only place that sees every read, so it is the only place these aggregates are
// correct). Timestamps are server-side epoch milliseconds; the reader clock is
// informational only.
type TagState struct {
	ReaderKey        string `json:"readerKey"`
	EPC              string `json:"epc"`
	Alias            string `json:"alias,omitempty"` // optional; asset-name resolution is a follow-up
	CapturePointName string `json:"capturePointName"`
	AntennaPort      int    `json:"antennaPort"` // most recent
	FirstSeenMs      int64  `json:"firstSeen"`
	LastSeenMs       int64  `json:"lastSeen"`
	ReadCount        int64  `json:"readCount"`
	LastRSSI         int    `json:"lastRssi"`
	RSSIAvg          int    `json:"rssiAvg"`
	RSSIMin          int    `json:"rssiMin"`
	RSSIMax          int    `json:"rssiMax"`
}

// orgEvent is a presence transition destined for one org's subscribers. For
// enter/update, tag is the full current state; for leave, only ReaderKey+EPC
// are populated.
type orgEvent struct {
	orgID int
	typ   EventType
	tag   TagState
}

// tagAgg is the live aggregate behind a TagState, plus coalescing bookkeeping.
type tagAgg struct {
	state    TagState
	rssiSum  int64     // running sum for the average
	dirty    bool      // re-sighted since the last emitted enter/update
	lastEmit time.Time // last time an enter/update was emitted for this tag
}

// store is the pure presence state machine: per-org, per-(reader,epc) tag
// aggregates with ENTER on first sight, coalesced UPDATE on re-sight, and LEAVE
// on expiry. It owns no goroutines, channels, or clock — callers pass `now` and
// drive flush/sweep on their own cadence, which makes it deterministically
// testable. Concurrency is the wrapping Tracker's responsibility.
type store struct {
	ttl      time.Duration // sliding window: stale when now-lastSeen >= ttl
	coalesce time.Duration // minimum interval between UPDATEs for a tag
	orgs     map[int]map[string]*tagAgg
}

func newStore(ttl, coalesce time.Duration) *store {
	return &store{ttl: ttl, coalesce: coalesce, orgs: make(map[int]map[string]*tagAgg)}
}

func tagKey(readerKey, epc string) string { return readerKey + "\x00" + epc }

// ingest records one read. On the first sight of a (reader,epc) it inserts the
// tag and returns an ENTER immediately; on a re-sight it folds the read into the
// aggregates and returns nil — the UPDATE is emitted later by flush (coalesced),
// so a continuously-present tag does not firehose the stream.
func (s *store) ingest(orgID int, readerKey string, r scanread.Read, now time.Time) []orgEvent {
	om := s.orgs[orgID]
	if om == nil {
		om = make(map[string]*tagAgg)
		s.orgs[orgID] = om
	}

	key := tagKey(readerKey, r.EPC)
	nowMs := now.UnixMilli()

	a := om[key]
	if a == nil {
		a = &tagAgg{
			state: TagState{
				ReaderKey:        readerKey,
				EPC:              r.EPC,
				CapturePointName: r.CapturePointName,
				AntennaPort:      r.AntennaPort,
				FirstSeenMs:      nowMs,
				LastSeenMs:       nowMs,
				ReadCount:        1,
				LastRSSI:         r.RSSI,
				RSSIAvg:          r.RSSI,
				RSSIMin:          r.RSSI,
				RSSIMax:          r.RSSI,
			},
			rssiSum:  int64(r.RSSI),
			lastEmit: now,
		}
		om[key] = a
		return []orgEvent{{orgID: orgID, typ: eventEnter, tag: a.state}}
	}

	a.state.ReadCount++
	a.rssiSum += int64(r.RSSI)
	a.state.LastRSSI = r.RSSI
	if r.RSSI < a.state.RSSIMin {
		a.state.RSSIMin = r.RSSI
	}
	if r.RSSI > a.state.RSSIMax {
		a.state.RSSIMax = r.RSSI
	}
	a.state.RSSIAvg = int(a.rssiSum / a.state.ReadCount)
	a.state.AntennaPort = r.AntennaPort
	a.state.CapturePointName = r.CapturePointName
	a.state.LastSeenMs = nowMs
	a.dirty = true
	return nil
}

// flush emits a coalesced UPDATE for every tag that has been re-sighted since
// its last emit and whose coalesce interval has elapsed. Render rate is thereby
// decoupled from read rate.
func (s *store) flush(now time.Time) []orgEvent {
	var out []orgEvent
	for orgID, om := range s.orgs {
		for _, a := range om {
			if a.dirty && now.Sub(a.lastEmit) >= s.coalesce {
				a.dirty = false
				a.lastEmit = now
				out = append(out, orgEvent{orgID: orgID, typ: eventUpdate, tag: a.state})
			}
		}
	}
	return out
}

// sweep evicts every tag past the sliding window and returns one LEAVE per
// evicted tag. Callers must sweep on a cadence well under ttl so LEAVE latency
// stays a fraction of the window (not up to 2x, the classic period==threshold
// bug).
func (s *store) sweep(now time.Time) []orgEvent {
	var out []orgEvent
	for orgID, om := range s.orgs {
		for key, a := range om {
			if now.Sub(time.UnixMilli(a.state.LastSeenMs)) >= s.ttl {
				delete(om, key)
				out = append(out, orgEvent{
					orgID: orgID,
					typ:   eventLeave,
					tag:   TagState{ReaderKey: a.state.ReaderKey, EPC: a.state.EPC},
				})
			}
		}
		if len(om) == 0 {
			delete(s.orgs, orgID)
		}
	}
	return out
}

// snapshot returns the current presence set for one org (the SSE seed sent on
// connect and as the periodic keyframe).
func (s *store) snapshot(orgID int) []TagState {
	om := s.orgs[orgID]
	out := make([]TagState, 0, len(om))
	for _, a := range om {
		out = append(out, a.state)
	}
	return out
}

// uniqueTags is the count of tags currently present for an org (footer stat).
func (s *store) uniqueTags(orgID int) int { return len(s.orgs[orgID]) }
