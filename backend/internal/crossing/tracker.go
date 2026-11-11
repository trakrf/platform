// Package crossing works out which way an asset passed a reader from the order
// of the strongest RSSI reads across that reader's antennas.
package crossing

import (
	"maps"
	"slices"
	"time"
)

// usable reports whether a read has a real signal strength. RSSI is negative
// dBm, and 0 is the parser's "no usable RSSI" value: in a signed comparison it
// would beat every real read.
func usable(rssi int) bool { return rssi < 0 }

// beats reports whether a is stronger than b by at least margin dB. Closer to
// zero is stronger, so compare as signed integers, never as absolute values:
// beats(-10, -19, 3) is true because -10 >= -16.
func beats(a, b, margin int) bool { return a >= b+margin }

// Read is one resolved read. LocationID is nil when its scan point has no location.
type Read struct {
	AssetID, ScanPointID int
	LocationID           *int
	RSSI                 int
}

// Seed is what the caller knows about an asset the tracker has no state for.
type Seed struct {
	Known      bool // the asset has a last known location
	LocationID int
	AtReader   bool // that location is one of this reader's
}

// Crossing is an asset moving between two of a reader's locations. ScanPointID
// is the read that settled it.
type Crossing struct {
	AssetID, From, To, ScanPointID int
	// Cold marks a crossing that came from a Seed rather than tracked state.
	Cold bool
}

// Result is the outcome of one message. Pass it to Apply once the crossings are
// safely recorded; dropping it leaves the tracker as it was.
type Result struct {
	Crossings []Crossing
	// Deduped counts repeat crossings that were not recorded.
	Deduped int
	// Ties counts runs of tied reads, not the reads themselves.
	Ties int
	// ColdCrossings counts the Crossings that came from a Seed.
	ColdCrossings int

	next map[stateKey]*assetState
}

type stateKey struct{ org, asset, reader int }

// route is the from and to locations a crossing is deduplicated on.
type route struct{ from, to int }

type recentRead struct {
	at          time.Time
	rssi        int
	scanPointID int
}

// assetState always has a leader: it is created by the first usable read.
type assetState struct {
	leader int
	// recent holds usable reads inside the window, per location. Antennas on one
	// location share an entry, so they count as one location.
	recent map[int][]recentRead
	// recorded is when each route last produced a crossing. A deduplicated repeat
	// does not touch it, or a flapping asset could never alert again.
	recorded map[route]time.Time
	// lastTie is the latest tied read of the current run, zero when there is none.
	lastTie time.Time
}

func newAssetState(leader int) *assetState {
	return &assetState{
		leader:   leader,
		recent:   map[int][]recentRead{},
		recorded: map[route]time.Time{},
	}
}

func (s *assetState) clone() *assetState {
	c := *s
	c.recent = make(map[int][]recentRead, len(s.recent))
	for loc, reads := range s.recent {
		c.recent[loc] = slices.Clone(reads)
	}
	c.recorded = maps.Clone(s.recorded)
	return &c
}

// observe drops reads older than the window, then adds this one.
func (s *assetState) observe(loc int, at time.Time, rssi, scanPointID int, window time.Duration) {
	for l, reads := range s.recent {
		reads = slices.DeleteFunc(reads, func(r recentRead) bool { return at.Sub(r.at) > window })
		if len(reads) == 0 {
			delete(s.recent, l)
			continue
		}
		s.recent[l] = reads
	}
	s.recent[loc] = append(s.recent[loc], recentRead{at: at, rssi: rssi, scanPointID: scanPointID})
}

// strongest returns a location's strongest recent read. Callers must first check
// the location has one: the zero read has RSSI 0, which beats any real read.
func (s *assetState) strongest(loc int) recentRead {
	return slices.MaxFunc(s.recent[loc], func(a, b recentRead) int { return a.rssi - b.rssi })
}

// Tracker holds the leader and recent reads for each (org, asset, reader). State
// is not evicted: it is bounded by assets × readers.
//
// It has no lock. Messages are handled one at a time, and a Result is computed
// against the state its Apply replaces, so calls must not overlap anyway.
type Tracker struct {
	cfg    Config
	states map[stateKey]*assetState
}

// NewTracker returns a Tracker with no state.
func NewTracker(cfg Config) *Tracker {
	return &Tracker{cfg: cfg, states: map[stateKey]*assetState{}}
}

// NeedsSeed lists, without repeats, the assets with no state at this reader.
func (t *Tracker) NeedsSeed(orgID, readerID int, assetIDs []int) []int {
	var missing []int
	for _, id := range assetIDs {
		if _, ok := t.states[stateKey{org: orgID, asset: id, reader: readerID}]; !ok && !slices.Contains(missing, id) {
			missing = append(missing, id)
		}
	}
	return missing
}

// Evaluate runs one message's reads, which must already be in order, and returns
// what they mean. at is the server time, used for the window and the dedup. seeds
// covers the assets from NeedsSeed. Evaluate does not change the tracker; only
// Apply does.
func (t *Tracker) Evaluate(orgID, readerID int, at time.Time, reads []Read, seeds map[int]Seed) Result {
	res := Result{next: map[stateKey]*assetState{}}
	for _, r := range reads {
		if r.LocationID == nil || !usable(r.RSSI) {
			continue
		}
		key := stateKey{org: orgID, asset: r.AssetID, reader: readerID}
		if s, ok := res.next[key]; ok {
			t.step(&res, s, at, r)
		} else if s, ok := t.states[key]; ok {
			s = s.clone()
			res.next[key] = s
			t.step(&res, s, at, r)
		} else {
			res.next[key] = t.coldStart(&res, at, r, seeds[r.AssetID])
		}
	}
	return res
}

// Apply makes a Result's state current.
func (t *Tracker) Apply(r Result) {
	maps.Copy(t.states, r.next)
}

// step handles a read for an asset that already has a leader.
func (t *Tracker) step(res *Result, s *assetState, at time.Time, r Read) {
	loc := *r.LocationID
	s.observe(loc, at, r.RSSI, r.ScanPointID, t.cfg.Window)

	switch {
	case loc == s.leader:
	case len(s.recent[s.leader]) == 0:
		t.cross(res, s, at, r.AssetID, loc, s.strongest(loc).scanPointID)
	case beats(s.strongest(loc).rssi, s.strongest(s.leader).rssi, t.cfg.MarginDB):
		// The crossing is settled by the strongest recent read at the new
		// location, which may not be the read that triggered the check.
		t.cross(res, s, at, r.AssetID, loc, s.strongest(loc).scanPointID)
	case beats(s.strongest(s.leader).rssi, s.strongest(loc).rssi, t.cfg.MarginDB):
		// The leader is clearly stronger: not a tie, nothing to report.
	default:
		// Neither side wins by the margin. A run of tied reads is one event: it
		// ends with a crossing, or once a whole window passes with no tied read.
		if s.lastTie.IsZero() || at.Sub(s.lastTie) > t.cfg.Window {
			res.Ties++
		}
		s.lastTie = at
	}
}

// coldStart handles the first read of an asset at a reader. Only a last known
// location on this reader is evidence of a pass in progress. Anything else makes
// the read's location the leader quietly, so a restart does not re-alert on every
// parked asset.
func (t *Tracker) coldStart(res *Result, at time.Time, r Read, seed Seed) *assetState {
	loc := *r.LocationID
	midPass := seed.Known && seed.LocationID != loc && seed.AtReader

	leader := loc
	if midPass {
		leader = seed.LocationID
	}
	s := newAssetState(leader)
	s.observe(loc, at, r.RSSI, r.ScanPointID, t.cfg.Window)
	if midPass {
		t.cross(res, s, at, r.AssetID, loc, r.ScanPointID)
		res.Crossings[len(res.Crossings)-1].Cold = true
		res.ColdCrossings++
	}
	return s
}

// cross moves the leader to a location, and records a crossing unless it
// repeats a recent one. scanPointID is the read that settled it.
func (t *Tracker) cross(res *Result, s *assetState, at time.Time, assetID, to, scanPointID int) {
	rt := route{from: s.leader, to: to}
	s.leader = rt.to
	s.lastTie = time.Time{}

	if last, ok := s.recorded[rt]; ok && at.Sub(last) <= t.cfg.Dedup {
		res.Deduped++
		return
	}
	s.recorded[rt] = at
	res.Crossings = append(res.Crossings, Crossing{
		AssetID: assetID, From: rt.from, To: rt.to, ScanPointID: scanPointID,
	})
}
