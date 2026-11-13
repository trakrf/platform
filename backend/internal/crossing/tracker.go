// Package crossing works out which way an asset passed a reader from a swap in
// heard signal strength across that reader's antennas: the leading location
// fading below its own recent peak as another grows above its own recent trough.
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
// zero is stronger, so compare as signed numbers, never as absolute values:
// beats(-10, -19, 3) is true because -10 >= -16.
func beats(a, b float64, margin int) bool { return a >= b+float64(margin) }

// Read is one resolved read. LocationID is nil when its scan point has no location.
type Read struct {
	AssetID, ScanPointID int
	LocationID           *int
	RSSI                 int
	// At is the reader's timestamp for the read, or the server receive time when
	// the reader sent none or one too far from it. It is never after the receive
	// time. Windows, growth, silence, gaps and the dedup all run on it.
	At time.Time
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
}

// Result is the outcome of one message. Pass it to Apply once the crossings are
// safely recorded; dropping it leaves the tracker as it was.
type Result struct {
	Crossings []Crossing
	// Deduped counts repeat crossings that were not recorded.
	Deduped int
	// Ties counts runs of tied reads, not the reads themselves.
	Ties int

	next map[stateKey]*assetState
}

type stateKey struct{ org, asset, reader int }

// readerKey identifies one reader.
type readerKey struct{ org, reader int }

// route is the from and to locations a crossing is deduplicated on.
type route struct{ from, to int }

// mark orders one asset's events: by reader time, then by arrival within it, so
// reads that share a timestamp still have an order.
type mark struct {
	at  time.Time
	seq uint64
}

func (m mark) after(o mark) bool {
	return m.at.After(o.at) || (m.at.Equal(o.at) && m.seq > o.seq)
}

type histRead struct {
	mark
	rssi        int
	scanPointID int
}

// growth is when a non-leader location last grew, and the read that would
// settle a crossing to it.
type growth struct {
	mark
	scanPointID int
}

// assetState is one asset at one reader. It always has a leader: it is created
// by the first usable read.
type assetState struct {
	leader int
	// clock is the latest read time. It never moves backwards: windows are
	// measured from it, and a read stamped earlier takes its place by stamp.
	clock time.Time
	seq   uint64
	// reset is the first read, or the last one to end a gap. The silence clock
	// starts here, and reads before it do not count towards a sustained level.
	reset mark
	// reads holds each location's reads of the last History, oldest first.
	// Antennas on one location share an entry, so they count as one location.
	reads map[int][]histRead
	// heard is when each location was last heard. A leader taken from a Seed has
	// no entry until it is heard; its silence clock runs from reset.
	heard map[int]mark
	// grew is the evidence the silence check acts on: growth since the leader was
	// last heard.
	grew map[int]growth
	// recorded is when each route last produced a crossing. A deduplicated repeat
	// does not touch it, or a flapping asset could never alert again.
	recorded map[route]time.Time
	// lastTie is the latest tied read of the current run, zero when there is none.
	lastTie time.Time
}

func newAssetState(leader int) *assetState {
	return &assetState{
		leader:   leader,
		reads:    map[int][]histRead{},
		heard:    map[int]mark{},
		grew:     map[int]growth{},
		recorded: map[route]time.Time{},
	}
}

func (s *assetState) clone() *assetState {
	c := *s
	c.reads = make(map[int][]histRead, len(s.reads))
	for loc, reads := range s.reads {
		c.reads[loc] = slices.Clone(reads)
	}
	c.heard = maps.Clone(s.heard)
	c.grew = maps.Clone(s.grew)
	c.recorded = maps.Clone(s.recorded)
	return &c
}

// admit places a read in time and returns its mark. A read older than History
// before the clock is dropped. One that ends a gap, or is the first, resets the
// silence clock and the growth evidence.
func (s *assetState) admit(at time.Time, cfg Config) (mark, bool) {
	switch {
	case s.clock.IsZero() || at.Sub(s.clock) > cfg.Gap:
		s.seq++
		s.reset = mark{at: at, seq: s.seq}
		clear(s.grew)
		s.clock = at
	case at.After(s.clock):
		s.clock = at
	case s.clock.Sub(at) > cfg.History:
		return mark{}, false
	}
	s.seq++
	return mark{at: at, seq: s.seq}, true
}

// prune drops the reads that have left History.
func (s *assetState) prune(history time.Duration) {
	for loc, reads := range s.reads {
		n := 0
		for n < len(reads) && s.clock.Sub(reads[n].at) > history {
			n++
		}
		if n == len(reads) {
			delete(s.reads, loc)
		} else if n > 0 {
			s.reads[loc] = reads[n:]
		}
	}
}

// insert keeps a location's reads in time order.
func (s *assetState) insert(loc int, r histRead) {
	reads := append(s.reads[loc], r)
	for i := len(reads) - 1; i > 0 && reads[i-1].after(reads[i].mark); i-- {
		reads[i-1], reads[i] = reads[i], reads[i-1]
	}
	s.reads[loc] = reads
}

func (s *assetState) hear(loc int, m mark) {
	if h, ok := s.heard[loc]; !ok || m.after(h) {
		s.heard[loc] = m
	}
}

// level is what a location's reads say at the clock.
type level struct {
	now       float64 // mean RSSI in the window
	strongest int     // the scan point of the strongest read in the window
	heard     int     // reads in the window
	sustained int     // of those, reads since the reset
	refs      int     // reads older than the window
	// peak and trough are the second-highest and second-lowest refs, so one
	// spike or one dropout cannot set them. Valid when refs is at least 2.
	peak, trough int
}

func (s *assetState) level(loc int, cfg Config) level {
	var lv level
	hi, lo := [2]int{}, [2]int{}
	sum, best := 0, 0
	for _, r := range s.reads[loc] {
		if s.clock.Sub(r.at) > cfg.Window {
			lv.refs++
			switch {
			case lv.refs == 1:
				hi, lo = [2]int{r.rssi, r.rssi}, [2]int{r.rssi, r.rssi}
			case r.rssi > hi[0]:
				hi = [2]int{r.rssi, hi[0]}
			case lv.refs == 2 || r.rssi > hi[1]:
				hi[1] = r.rssi
			}
			switch {
			case lv.refs == 1:
			case r.rssi < lo[0]:
				lo = [2]int{r.rssi, lo[0]}
			case lv.refs == 2 || r.rssi < lo[1]:
				lo[1] = r.rssi
			}
			continue
		}
		if lv.heard == 0 || r.rssi > best {
			best, lv.strongest = r.rssi, r.scanPointID
		}
		lv.heard++
		sum += r.rssi
		if r.after(s.reset) {
			lv.sustained++
		}
	}
	if lv.heard > 0 {
		lv.now = float64(sum) / float64(lv.heard)
	}
	lv.peak, lv.trough = hi[1], lo[1]
	return lv
}

// grew reports whether a location is sustained and rising: at least MinReads in
// the window, and a level at least GrowDB above its recent trough, or no trough
// to judge against.
func (lv level) grew(cfg Config) bool {
	return lv.sustained >= cfg.MinReads && (lv.refs < 2 || lv.now-float64(lv.trough) >= float64(cfg.GrowDB))
}

// readsAfter counts a location's reads after a mark.
func (s *assetState) readsAfter(loc int, m mark) int {
	reads := s.reads[loc]
	n := 0
	for n < len(reads) && reads[len(reads)-1-n].after(m) {
		n++
	}
	return n
}

// Tracker holds the leader and recent reads for each (org, asset, reader). State
// is not evicted: it is bounded by assets × readers.
//
// It has no lock. Messages are handled one at a time, and a Result is computed
// against the state its Apply replaces, so calls must not overlap anyway.
type Tracker struct {
	cfg    Config
	states map[stateKey]*assetState
	// waiting indexes, per reader, the assets holding growth evidence: the only
	// ones a silence check can move, so a message need not scan every state.
	waiting map[readerKey]map[int]bool
}

// NewTracker returns a Tracker with no state.
func NewTracker(cfg Config) *Tracker {
	return &Tracker{cfg: cfg, states: map[stateKey]*assetState{}, waiting: map[readerKey]map[int]bool{}}
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

// Evaluate runs one message's reads, which must already be in reader-time
// order, and returns what they mean. seeds covers the assets from NeedsSeed. The
// silence check then runs for this reader's waiting assets at the message's
// latest read time, whether or not the message has a read for them. Evaluate
// does not change the tracker; only Apply does.
func (t *Tracker) Evaluate(orgID, readerID int, reads []Read, seeds map[int]Seed) Result {
	res := Result{next: map[stateKey]*assetState{}}
	var at time.Time
	for _, r := range reads {
		at = later(at, r.At)
		if r.LocationID == nil || !usable(r.RSSI) {
			continue
		}
		key := stateKey{org: orgID, asset: r.AssetID, reader: readerID}
		s, ok := res.next[key]
		if !ok {
			if live, known := t.states[key]; known {
				s = live.clone()
			} else {
				s = newAssetState(firstLeader(r, seeds[r.AssetID]))
			}
			res.next[key] = s
		}
		t.step(&res, s, r)
	}
	if !at.IsZero() {
		t.checkSilence(&res, orgID, readerID, at)
	}
	return res
}

// firstLeader is who leads an asset with no state, with no crossing: the last
// known location if it is one of this reader's, or the read's own. A restart
// therefore re-alerts for no parked asset, and the rule decides from there.
func firstLeader(r Read, seed Seed) int {
	if seed.Known && seed.AtReader {
		return seed.LocationID
	}
	return *r.LocationID
}

func later(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}

// Apply makes a Result's state current.
func (t *Tracker) Apply(r Result) {
	maps.Copy(t.states, r.next)
	for key, s := range r.next {
		rk := readerKey{org: key.org, reader: key.reader}
		if len(s.grew) == 0 {
			delete(t.waiting[rk], key.asset)
			continue
		}
		if t.waiting[rk] == nil {
			t.waiting[rk] = map[int]bool{}
		}
		t.waiting[rk][key.asset] = true
	}
}

// step handles one usable read.
func (t *Tracker) step(res *Result, s *assetState, r Read) {
	m, ok := s.admit(r.At, t.cfg)
	if !ok {
		return
	}
	loc := *r.LocationID
	s.prune(t.cfg.History)
	s.insert(loc, histRead{mark: m, rssi: r.RSSI, scanPointID: r.ScanPointID})
	s.hear(loc, m)
	if loc == s.leader {
		// Growth from before the leader's latest read can no longer count.
		for l, g := range s.grew {
			if !g.after(s.heard[loc]) {
				delete(s.grew, l)
			}
		}
		return
	}
	t.judge(res, s, r.AssetID, loc, m)
}

// judge applies the rule to a read at a location L other than the leader A.
// L growing is the first condition. With A heard, the crossing needs A faded
// below its own recent peak and L the margin above A; with A unheard, only the
// silence check can move it. Neither, with both heard and no winner, is a tie.
func (t *Tracker) judge(res *Result, s *assetState, assetID, loc int, m mark) {
	l, a := s.level(loc, t.cfg), s.level(s.leader, t.cfg)
	grew := l.grew(t.cfg)
	if grew {
		s.grew[loc] = growth{mark: m, scanPointID: l.strongest}
	}
	if a.heard == 0 {
		// The silence check runs at this read's time, as it would on any
		// other message, so a message spanning the silence behaves the same.
		if to, g, due := s.silenceTarget(s.clock, t.cfg); due {
			t.cross(res, s, s.clock, assetID, to, g.scanPointID)
		}
		return
	}
	if grew && a.refs >= 2 && float64(a.peak)-a.now >= float64(t.cfg.FadeDB) && beats(l.now, a.now, t.cfg.MarginDB) {
		t.cross(res, s, s.clock, assetID, loc, l.strongest)
		return
	}
	if l.heard > 0 && !beats(l.now, a.now, t.cfg.MarginDB) && !beats(a.now, l.now, t.cfg.MarginDB) {
		// Neither side wins by the margin. A run of tied reads is one event: it
		// ends with a crossing, or once a whole window passes with no tied read.
		if s.lastTie.IsZero() || s.clock.Sub(s.lastTie) > t.cfg.Window {
			res.Ties++
		}
		s.lastTie = s.clock
	}
}

// silenceTarget returns the location to cross to once the leader has been
// unheard for Silence, measured from the later of its last read and the last
// reset: the location that grew most recently since then, with at least
// MinReads reads since then. A leader taken from a Seed has never been heard,
// so its clock runs from the reset.
func (s *assetState) silenceTarget(now time.Time, cfg Config) (to int, g growth, ok bool) {
	since := s.reset
	if h, heard := s.heard[s.leader]; heard && h.after(since) {
		since = h
	}
	if now.Sub(since.at) < cfg.Silence {
		return 0, growth{}, false
	}
	for _, loc := range slices.Sorted(maps.Keys(s.grew)) {
		c := s.grew[loc]
		if c.after(since) && (!ok || c.after(g.mark)) && s.readsAfter(loc, since) >= cfg.MinReads {
			to, g, ok = loc, c, true
		}
	}
	return to, g, ok
}

// checkSilence runs the silence check for every asset at this reader holding
// growth evidence, at the message's latest read time, whether or not the message
// has a read for it. A later read of the asset is not needed, so a boat that was
// heard briefly and left is not lost.
//
// It only runs when a message reaches detection: one with only unresolved tags
// does not, because PersistReadsDetect skips detection when nothing resolved. A
// reader that sends nothing more leaves the check waiting.
func (t *Tracker) checkSilence(res *Result, orgID, readerID int, at time.Time) {
	ids := map[int]bool{}
	maps.Copy(ids, t.waiting[readerKey{org: orgID, reader: readerID}])
	for key, s := range res.next {
		if len(s.grew) > 0 {
			ids[key.asset] = true
		}
	}
	for _, id := range slices.Sorted(maps.Keys(ids)) {
		key := stateKey{org: orgID, asset: id, reader: readerID}
		s, pending := res.next[key]
		if !pending {
			s = t.states[key]
		}
		// The asset's own clock never moves backwards, and the message's time
		// is never ahead of the receive time, so neither is the check.
		now := later(at, s.clock)
		to, g, due := s.silenceTarget(now, t.cfg)
		if !due {
			continue
		}
		// Only the Result may change: a live state is cloned, and only if it fires.
		if !pending {
			s = s.clone()
			res.next[key] = s
		}
		t.cross(res, s, now, id, to, g.scanPointID)
	}
}

// cross moves the leader to a location, and records a crossing unless it repeats
// a recent one. The read history stays, so a noisy reversal cannot fire straight
// away; the new leader's silence clock starts here. scanPointID is the read
// that settled it.
func (t *Tracker) cross(res *Result, s *assetState, at time.Time, assetID, to, scanPointID int) {
	rt := route{from: s.leader, to: to}
	s.leader = rt.to
	s.lastTie = time.Time{}
	clear(s.grew)
	s.seq++
	s.hear(to, mark{at: at, seq: s.seq})

	if last, ok := s.recorded[rt]; ok && at.Sub(last) <= t.cfg.Dedup {
		res.Deduped++
		return
	}
	s.recorded[rt] = at
	res.Crossings = append(res.Crossings, Crossing{
		AssetID: assetID, From: rt.from, To: rt.to, ScanPointID: scanPointID,
	})
}
