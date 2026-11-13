package crossing

import (
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settle is a boat heard loud at A and quiet at B: two reads each, so both have
// references once a window has passed.
func settle() []timedRead {
	return []timedRead{
		{sec: 0, loc: locA, rssi: -40}, {sec: 0.1, loc: locA, rssi: -40},
		{sec: 0.2, loc: locB, rssi: -60}, {sec: 0.3, loc: locB, rssi: -60},
	}
}

// swap is the boat heard the other way round from sec: the quiet side first,
// then the loud one. Six seconds after the last phase every earlier read is a
// reference, so the loud side has grown and the quiet one faded.
func swap(sec float64, quiet, loud int) []timedRead {
	return []timedRead{
		{sec: sec, loc: quiet, rssi: -60}, {sec: sec + 0.1, loc: quiet, rssi: -60},
		{sec: sec + 0.2, loc: loud, rssi: -40}, {sec: sec + 0.3, loc: loud, rssi: -40},
	}
}

func seq(parts ...[]timedRead) []timedRead {
	var out []timedRead
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// every is one location read from 0s to before end, step apart, at one level.
func every(step, end float64, loc, rssi int) []timedRead {
	var reads []timedRead
	for sec := 0.0; sec < end; sec += step {
		reads = append(reads, timedRead{sec: sec, loc: loc, rssi: rssi})
	}
	return reads
}

func bySec(reads []timedRead) []timedRead {
	sort.SliceStable(reads, func(i, j int) bool { return reads[i].sec < reads[j].sec })
	return reads
}

// cycles plays an antenna cycle every 500 ms, each location read once in order,
// 125 ms apart. levels gives each location's RSSI for the cycle at sec.
func cycles(from, to float64, levels func(sec float64) map[int]int, order ...int) []timedRead {
	var reads []timedRead
	for sec := from; sec < to; sec += 0.5 {
		lv := levels(sec)
		for i, l := range order {
			reads = append(reads, timedRead{sec: sec + float64(i)*0.125, loc: l, rssi: lv[l]})
		}
	}
	return reads
}

func swapAt(sec float64, before, after map[int]int) func(float64) map[int]int {
	return func(s float64) map[int]int {
		if s < sec {
			return before
		}
		return after
	}
}

// readOrders is every order of A and B within a cycle.
var readOrders = map[string][]int{"A first": {locA, locB}, "B first": {locB, locA}}

// parkedBoat is a boat that does not move: each location has a level, read every
// period, with uniform noise, random drops and optional bursts of silence.
type parkedBoat struct {
	levels       []int // locA upward
	period, drop float64
	noise        int
	on, offMean  float64
}

func (p parkedBoat) reads(dur float64, rng *rand.Rand) []timedRead {
	var reads []timedRead
	for i, lv := range p.levels {
		next, offUntil := p.on, -1.0
		for t := float64(i) * p.period / float64(len(p.levels)); t < dur; t += p.period {
			if p.on > 0 && t >= next {
				offUntil = next + 3 + rng.ExpFloat64()*p.offMean
				next = offUntil + p.on
			}
			if t < offUntil || rng.Float64() < p.drop {
				continue
			}
			reads = append(reads, timedRead{sec: t, loc: locA + i, rssi: lv + rng.Intn(2*p.noise+1) - p.noise})
		}
	}
	return bySec(reads)
}

// parkedCrossings plays a parked boat for an hour, leader A, per seed.
func parkedCrossings(boat parkedBoat, seed int64) int {
	reads := boat.reads(3600, rand.New(rand.NewSource(seed)))
	return len(routesOf(play(NewTracker(DefaultConfig()), locA, reads, 5, 3600).crossings))
}

func TestTracker_ParkedDropoutsAreQuiet(t *testing.T) {
	for seed := int64(0); seed < 5; seed++ {
		assert.Zero(t, parkedCrossings(parkedBoat{levels: []int{-50, -52}, period: 0.5, drop: 0.8, noise: 2}, seed), "seed %d", seed)
	}
}

func TestTracker_ParkedBurstsAreQuiet(t *testing.T) {
	for seed := int64(0); seed < 5; seed++ {
		assert.Zero(t, parkedCrossings(parkedBoat{levels: []int{-50, -52}, period: 0.5, noise: 2, on: 10, offMean: 2}, seed), "seed %d", seed)
	}
}

func TestTracker_NoisyEqualParkedStaysLow(t *testing.T) {
	for seed := int64(0); seed < 5; seed++ {
		assert.LessOrEqual(t, parkedCrossings(parkedBoat{levels: []int{-50, -50}, period: 0.5, noise: 4}, seed), 2, "seed %d", seed)
	}
}

func TestTracker_LouderIsNotEnough(t *testing.T) {
	for name, order := range readOrders {
		t.Run(name, func(t *testing.T) {
			tr := NewTracker(DefaultConfig())
			got := play(tr, locA, cycles(0, 60, swapAt(0, nil, map[int]int{locA: -50, locB: -45}), order...), 5, 60)
			assert.Empty(t, routesOf(got.crossings))
			assert.Equal(t, locA, leaderOf(tr))
		})
	}
}

func TestTracker_OnlyOneSideMoves(t *testing.T) {
	for name, order := range readOrders {
		t.Run(name, func(t *testing.T) {
			tr := NewTracker(DefaultConfig())
			got := play(tr, locA, cycles(0, 120, swapAt(60,
				map[int]int{locA: -50, locB: -60},
				map[int]int{locA: -50, locB: -54}), order...), 5, 120)
			assert.Empty(t, routesOf(got.crossings))
			assert.Equal(t, locA, leaderOf(tr))
		})
	}
}

// A steady at -50 with one -35 read. The peak is the second-highest reference,
// so when B grows and beats A, A has not faded from -35: no crossing.
func TestTracker_OneSpikeIsNotAPeak(t *testing.T) {
	reads := every(0.5, 30, locA, -50)
	reads[6].rssi = -35 // at 3s
	for sec := 0.25; sec < 30; sec += 0.5 {
		rssi := -60
		if sec >= 15 {
			rssi = -44
		}
		reads = append(reads, timedRead{sec: sec, loc: locB, rssi: rssi})
	}
	tr := NewTracker(DefaultConfig())
	got := play(tr, locA, bySec(reads), 5, 60)
	assert.Empty(t, routesOf(got.crossings))
	assert.Equal(t, locA, leaderOf(tr))
}

func TestTracker_SwapCrosses(t *testing.T) {
	for name, tc := range map[string]struct {
		from, to int
	}{"A to B": {locA, locB}, "B to A": {locB, locA}} {
		t.Run(name, func(t *testing.T) {
			tr := NewTracker(DefaultConfig())
			got := play(tr, tc.from, cycles(0, 60, swapAt(30,
				map[int]int{tc.from: -40, tc.to: -60},
				map[int]int{tc.from: -55, tc.to: -42}), locA, locB), 5, 60)
			assert.Equal(t, [][2]int{{tc.from, tc.to}}, routesOf(got.crossings))
			assert.Equal(t, tc.to, leaderOf(tr))
		})
	}
}

// Out through the gate and straight back: both crossings, in order, within 40s.
func TestTracker_QuickReturnCrossesBothWays(t *testing.T) {
	out, back := map[int]int{locA: -40, locB: -60}, map[int]int{locA: -55, locB: -42}
	reads := cycles(0, 20, swapAt(0, nil, out), locA, locB)
	reads = append(reads, cycles(20, 40, swapAt(0, nil, back), locA, locB)...)
	reads = append(reads, cycles(40, 60, swapAt(0, nil, out), locA, locB)...)
	tr := NewTracker(DefaultConfig())
	got := play(tr, locA, reads, 5, 100)
	assert.Equal(t, [][2]int{{locA, locB}, {locB, locA}}, routesOf(got.crossings))
	require.Len(t, got.crossedAt, 2)
	assert.Less(t, got.crossedAt[1]-got.crossedAt[0], 40.0)
}

func TestTracker_SlowPassCrosses(t *testing.T) {
	for seed := int64(0); seed < 5; seed++ {
		rng := rand.New(rand.NewSource(seed))
		var reads []timedRead
		for sec := 0.0; sec < 160; sec += 0.25 {
			f := math.Min(1, math.Max(0, (sec-20)/120))
			r := timedRead{sec: sec, loc: locA, rssi: int(math.Round(-45-20*f)) + rng.Intn(5) - 2}
			if int(sec*4)%2 == 1 {
				r.loc, r.rssi = locB, int(math.Round(-65+20*f))+rng.Intn(5)-2
			}
			reads = append(reads, r)
		}
		tr := NewTracker(DefaultConfig())
		got := play(tr, locA, reads, 5, 160)
		assert.Equal(t, [][2]int{{locA, locB}}, routesOf(got.crossings), "seed %d", seed)
	}
}

// A boat flapping between the antennas every 6s for over five minutes. Each flap
// moves the leader; a route is recorded once per Dedup, measured from the last
// recorded crossing, not the last suppressed one.
func TestTracker_RepeatRouteWithinDedupIsQuiet(t *testing.T) {
	flaps := func(n int) []timedRead {
		reads := settle()
		for k := 1; k <= n; k++ {
			quiet, loud := locA, locB
			if k%2 == 0 {
				quiet, loud = locB, locA
			}
			reads = append(reads, swap(6*float64(k), quiet, loud)...)
		}
		return reads
	}
	t.Run("a repeat within the dedup records nothing but the leader moves", func(t *testing.T) {
		tr := NewTracker(DefaultConfig())
		got := run(tr, flaps(3))
		assert.Equal(t, [][2]int{{locA, locB}, {locB, locA}}, routesOf(got.crossings))
		assert.Equal(t, 1, got.deduped)
		assert.Equal(t, locB, leaderOf(tr))
	})
	t.Run("the dedup expires from the recorded crossing", func(t *testing.T) {
		tr := NewTracker(DefaultConfig())
		got := run(tr, flaps(54))
		assert.Equal(t, [][2]int{{locA, locB}, {locB, locA}, {locA, locB}, {locB, locA}}, routesOf(got.crossings))
		assert.Equal(t, []float64{6.3, 12.3, 318.3, 324.3}, got.crossedAt)
		assert.Equal(t, 50, got.deduped)
		assert.Equal(t, locA, leaderOf(tr))
	})
}

// drive is a boat parked 20s before the first antenna, driven past antennas 10 m
// apart at speed, then parked 20s. Signal falls off with distance, exponent 2.5.
func drive(speed, period, drop float64, rng *rand.Rand) []timedRead {
	xs := []float64{0, 10, 20, 30}
	const x0, x1 = -4.0, 34.0
	move := (x1 - x0) / speed
	pos := func(t float64) float64 { return x0 + (x1-x0)*math.Min(1, math.Max(0, (t-20)/move)) }
	var reads []timedRead
	for i, ax := range xs {
		for t := float64(i) * period / 4; t < 40+move; t += period {
			if rng.Float64() < drop {
				continue
			}
			r := -40 - 25*math.Log10(math.Max(math.Abs(pos(t)-ax), 1)) + float64(rng.Intn(5)-2)
			if r >= -80 {
				reads = append(reads, timedRead{sec: t, loc: locA + i, rssi: int(math.Round(r))})
			}
		}
	}
	return bySec(reads)
}

// The boat is read, densely or patchily, for as long as other tags are: a boat
// whose reads all stop while the yard's go on has left, and is another case.
func TestTracker_FourAntennas(t *testing.T) {
	route := [][2]int{{locA, locB}, {locB, locC}, {locC, locD}}
	for seed := int64(0); seed < 20; seed++ {
		dense := play(NewTracker(DefaultConfig()), locA, drive(2, 0.5, 0, rand.New(rand.NewSource(seed))), 5, 60)
		assert.Equal(t, route, routesOf(dense.crossings), "dense, seed %d", seed)

		patchy := play(NewTracker(DefaultConfig()), locA, drive(2, 0.5, 0.7, rand.New(rand.NewSource(seed))), 5, 60)
		for _, r := range routesOf(patchy.crossings) {
			assert.Greater(t, r[1], r[0], "patchy, seed %d: backward route", seed)
		}
	}
}

// A steady, then a B-only pass of two reads, then nothing from this boat. The
// silence check fires once A has been unheard 30s, on another tag's message.
func TestTracker_ShortPassThenGoneCrossesAfterSilence(t *testing.T) {
	reads := append(every(0.5, 10, locA, -45), timedRead{sec: 10, loc: locB, rssi: -50}, timedRead{sec: 10.5, loc: locB, rssi: -50})
	tr := NewTracker(DefaultConfig())
	got := play(tr, locA, reads, 0, 0)
	require.Empty(t, routesOf(got.crossings))

	early := tr.Evaluate(testOrg, testReader, []Read{otherTag(39)}, nil)
	assert.Empty(t, routesOf(early.Crossings), "A last heard at 9.5s: 29.5s of silence")

	res := tr.Evaluate(testOrg, testReader, []Read{otherTag(39.5)}, nil)
	assert.Equal(t, []Crossing{crossing(locA, locB, scanPoint(locB, 0))}, res.Crossings)
	tr.Apply(res)
	assert.Equal(t, locB, leaderOf(tr))
	assert.Empty(t, tr.Evaluate(testOrg, testReader, []Read{otherTag(45)}, nil).Crossings, "fires only once")
}
