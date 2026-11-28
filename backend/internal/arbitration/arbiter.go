// Package arbitration decides which location an asset is at when several
// gateways hear it at once: the strongest one, with a margin and a dwell so the
// answer does not flap. Without it the first gateway message of each minute
// sets the stored location, which is a race, not a measurement.
//
// It is keyed by scan point and knows nothing about device types; the caller
// decides which reads are arbitrated (Config.Enabled).
package arbitration

import (
	"maps"
	"sort"
	"sync"
	"time"
)

// usable reports whether a read has a real signal strength. RSSI is negative
// dBm, and 0 is the parser's "no usable RSSI" value: in a signed comparison it
// would beat every real read. Same rule as crossing.
func usable(rssi int) bool { return rssi < 0 }

// beats reports whether a is stronger than b by at least margin dB. Closer to
// zero is stronger, so compare signed: beats(-40, -50, 6) is true.
func beats(a, b float64, margin int) bool { return a >= b+float64(margin) }

// Read is one resolved read. At is the server receive time: every gateway's
// reads are judged on one clock.
type Read struct {
	AssetID, ScanPointID int
	LocationID           *int
	RSSI                 int
	At                   time.Time
}

// Seed is an asset's last stored location, for an asset the arbiter holds no
// state for (a restart, or one idle long enough to be evicted).
type Seed struct{ LocationID int }

// Assignment is where an asset is after a message. ScanPointID is the strongest
// scan point of that location, or 0 while a seeded location has not been heard
// since the seed. Switched is true when this message changed the location,
// including an asset's first assignment.
type Assignment struct {
	AssetID, LocationID, ScanPointID int
	Switched                         bool
}

// Result is the outcome of one message. Pass it to Apply once the scan has
// committed; dropping it leaves the arbiter as it was.
type Result struct {
	Assignments map[int]Assignment
	org         int
	next        map[int]*assetState
	now         time.Time
}

type stateKey struct{ org, asset int }

type sample struct {
	at   time.Time
	rssi int
}

type pointState struct {
	location int
	samples  []sample // usable reads in the window, oldest first
}

type assetState struct {
	points       map[int]*pointState // by scan point
	location     int                 // assigned location; 0 = none
	scanPoint    int                 // its strongest scan point; 0 = not yet heard
	lastHeard    time.Time           // location last heard, assigned or seeded
	lastLevel    float64             // location's level when last heard
	hasLastLevel bool                // false for a seed not yet heard
	challenger   int                 // location leading by the margin; 0 = none
	leadSince    time.Time
	prevLocation int       // location assigned before this one; 0 = none
	leftAt       time.Time // when the asset left prevLocation
	lastRead     time.Time
}

func (s *assetState) clone() *assetState {
	c := *s
	c.points = make(map[int]*pointState, len(s.points))
	for id, p := range s.points {
		c.points[id] = &pointState{location: p.location, samples: append([]sample(nil), p.samples...)}
	}
	return &c
}

// Arbiter holds per-asset arbitration state. It is safe for concurrent use.
type Arbiter struct {
	cfg       Config
	mu        sync.Mutex
	states    map[stateKey]*assetState
	lastSweep time.Time
}

func New(cfg Config) *Arbiter {
	return &Arbiter{cfg: cfg, states: map[stateKey]*assetState{}}
}

// NeedsSeed returns the assets, of those given, the arbiter holds no state for.
func (a *Arbiter) NeedsSeed(org int, assetIDs []int) []int {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []int
	seen := map[int]bool{}
	for _, id := range assetIDs {
		if _, ok := a.states[stateKey{org, id}]; !ok && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// Evaluate folds one message's reads into copies of the assets' state, in time
// order, and returns each asset's assignment. It changes nothing until Apply.
// An asset with no usable read and no prior or seeded location gets no
// assignment.
func (a *Arbiter) Evaluate(org int, reads []Read, seeds map[int]Seed) Result {
	ordered := append([]Read(nil), reads...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].At.Before(ordered[j].At) })

	res := Result{Assignments: map[int]Assignment{}, org: org, next: map[int]*assetState{}}
	a.mu.Lock()
	for _, r := range ordered {
		if _, ok := res.next[r.AssetID]; ok {
			continue
		}
		if st, ok := a.states[stateKey{org, r.AssetID}]; ok {
			res.next[r.AssetID] = st.clone()
			continue
		}
		st := &assetState{points: map[int]*pointState{}}
		if s, ok := seeds[r.AssetID]; ok && s.LocationID != 0 {
			st.location, st.lastHeard = s.LocationID, r.At
		}
		res.next[r.AssetID] = st
	}
	a.mu.Unlock()

	switched := map[int]bool{}
	for _, r := range ordered {
		st := res.next[r.AssetID]
		if r.At.After(st.lastRead) {
			st.lastRead = r.At
		}
		if r.At.After(res.now) {
			res.now = r.At
		}
		if !usable(r.RSSI) || r.LocationID == nil {
			continue
		}
		p, ok := st.points[r.ScanPointID]
		if !ok || p.location != *r.LocationID {
			p = &pointState{location: *r.LocationID}
			st.points[r.ScanPointID] = p
		}
		p.samples = append(p.samples, sample{at: r.At, rssi: r.RSSI})
		if a.decide(st, r.At) {
			switched[r.AssetID] = true
		}
	}

	for id, st := range res.next {
		if st.location == 0 {
			continue
		}
		res.Assignments[id] = Assignment{AssetID: id, LocationID: st.location, ScanPointID: st.scanPoint, Switched: switched[id]}
	}
	return res
}

// level is a location's strength now: the best window mean of its scan points.
type level struct {
	mean      float64
	scanPoint int
	last      time.Time // newest read of any of its scan points
}

// levels prunes samples older than the window and returns each heard location's
// level.
func (a *Arbiter) levels(st *assetState, now time.Time) map[int]level {
	cutoff := now.Add(-a.cfg.Window)
	out := map[int]level{}
	for id, p := range st.points {
		i := 0
		for i < len(p.samples) && !p.samples[i].at.After(cutoff) {
			i++
		}
		p.samples = p.samples[i:]
		if len(p.samples) == 0 {
			delete(st.points, id)
			continue
		}
		sum := 0
		for _, s := range p.samples {
			sum += s.rssi
		}
		mean := float64(sum) / float64(len(p.samples))
		newest := p.samples[len(p.samples)-1].at
		cur, ok := out[p.location]
		if !ok || mean > cur.mean || (mean == cur.mean && id < cur.scanPoint) {
			cur.mean, cur.scanPoint = mean, id
		}
		if newest.After(cur.last) {
			cur.last = newest
		}
		out[p.location] = cur
	}
	return out
}

// decide applies the rules at time now and reports whether the location changed.
func (a *Arbiter) decide(st *assetState, now time.Time) bool {
	lv := a.levels(st, now)

	// The strongest location other than the assigned one.
	challenger, best := 0, level{}
	for loc, l := range lv {
		if loc == st.location {
			continue
		}
		if challenger == 0 || l.mean > best.mean || (l.mean == best.mean && loc < challenger) {
			challenger, best = loc, l
		}
	}

	if st.location == 0 {
		if challenger == 0 {
			return false
		}
		a.assign(st, challenger, best.scanPoint, now)
		st.lastLevel, st.hasLastLevel = best.mean, true
		return true
	}

	inc, heard := lv[st.location]
	if heard {
		st.scanPoint, st.lastHeard = inc.scanPoint, inc.last
		st.lastLevel, st.hasLastLevel = inc.mean, true
	}
	if challenger == 0 {
		st.challenger = 0
		return false
	}

	// Going straight back to the location just left is how a still asset at a
	// boundary looks: two rooms hear it about equally, and each gateway's
	// routine report gaps hand it back and forth. A walk does not go straight
	// back. So a return holds its lead for ReturnDwell, and a gap alone never
	// sends the asset back.
	returning := a.cfg.ReturnWindow > 0 && challenger == st.prevLocation && now.Sub(st.leftAt) < a.cfg.ReturnWindow

	// Unheard is not gone: a gateway can skip a tag it hears well for minutes.
	// The unheard location yields at once only to one heard about as strongly as
	// it last was, which means the asset turned up somewhere comparable. Anything
	// weaker waits out Silence. A seed has no level, and counts as heard when it
	// is made, so a restart never hands the asset to whichever gateway happens
	// to publish first.
	if !heard {
		st.challenger = 0
		if now.Sub(st.lastHeard) >= a.cfg.Silence ||
			(!returning && st.hasLastLevel && beats(best.mean, st.lastLevel, -a.cfg.MarginDB)) {
			a.assign(st, challenger, best.scanPoint, now)
			st.lastLevel, st.hasLastLevel = best.mean, true
			return true
		}
		return false
	}

	if !beats(best.mean, inc.mean, a.cfg.MarginDB) {
		st.challenger = 0
		return false
	}
	if st.challenger != challenger {
		st.challenger, st.leadSince = challenger, now
	}
	dwell := a.cfg.Dwell
	if returning {
		dwell = a.cfg.ReturnDwell
	}
	if now.Sub(st.leadSince) >= dwell {
		a.assign(st, challenger, best.scanPoint, now)
		st.lastLevel, st.hasLastLevel = best.mean, true
		return true
	}
	return false
}

// assign makes location the asset's, heard now from scanPoint.
func (a *Arbiter) assign(st *assetState, location, scanPoint int, now time.Time) {
	if st.location != 0 && st.location != location {
		st.prevLocation, st.leftAt = st.location, now
	}
	st.location, st.scanPoint, st.lastHeard = location, scanPoint, now
	st.challenger = 0
}

// Apply commits a Result. It also evicts assets with no read at all for
// Silence: by then a re-seed from the stored location decides exactly as their
// state would have.
func (a *Arbiter) Apply(res Result) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for id, st := range res.next {
		a.states[stateKey{res.org, id}] = st
	}
	if res.now.Sub(a.lastSweep) < a.cfg.Window {
		return
	}
	a.lastSweep = res.now
	maps.DeleteFunc(a.states, func(_ stateKey, st *assetState) bool {
		return res.now.Sub(st.lastRead) > a.cfg.Silence
	})
}

// Len is the number of assets held, for the state gauge.
func (a *Arbiter) Len() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.states)
}
