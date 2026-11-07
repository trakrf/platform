package crossing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testOrg    = 1
	testReader = 7
	testAsset  = 42

	locA = 1
	locB = 2
	locC = 3
	// noLoc marks a read from a scan point that has no location.
	noLoc = 0
)

var epoch = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func secs(s float64) time.Time {
	return epoch.Add(time.Duration(s * float64(time.Second)))
}

// timedRead is one read delivered as its own message.
type timedRead struct {
	sec  float64
	loc  int
	rssi int
	// antenna makes the scan point distinct among antennas on one location.
	antenna int
}

func (r timedRead) read() Read {
	out := Read{AssetID: testAsset, ScanPointID: scanPoint(r.loc, r.antenna), RSSI: r.rssi}
	if r.loc != noLoc {
		loc := r.loc
		out.LocationID = &loc
	}
	return out
}

func scanPoint(loc, antenna int) int { return loc*100 + antenna }

// totals sums the per-message results of a run.
type totals struct {
	crossings                  []Crossing
	deduped, ties, coldCrosses int
}

func (a *totals) add(r Result) {
	a.crossings = append(a.crossings, r.Crossings...)
	a.deduped += r.Deduped
	a.ties += r.Ties
	a.coldCrosses += r.ColdCrossings
}

func run(tr *Tracker, reads []timedRead) totals {
	var sum totals
	for _, r := range reads {
		res := tr.Evaluate(testOrg, testReader, secs(r.sec), []Read{r.read()}, nil)
		tr.Apply(res)
		sum.add(res)
	}
	return sum
}

func leaderOf(tr *Tracker) int {
	s := tr.states[stateKey{org: testOrg, asset: testAsset, reader: testReader}]
	if s == nil {
		return 0
	}
	return s.leader
}

func crossing(from, to, scanPointID int) Crossing {
	return Crossing{AssetID: testAsset, From: from, To: to, ScanPointID: scanPointID}
}

func TestTracker(t *testing.T) {
	aToB := crossing(locA, locB, scanPoint(locB, 0))
	bToA := crossing(locB, locA, scanPoint(locA, 0))

	tests := []struct {
		name       string
		reads      []timedRead
		want       []Crossing
		wantDedup  int
		wantTies   int
		wantLeader int
	}{
		{
			name: "spec example",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -52}, {sec: 1, loc: locA, rssi: -40},
				{sec: 1.5, loc: locB, rssi: -58}, {sec: 2.5, loc: locB, rssi: -44},
				{sec: 3, loc: locA, rssi: -60}, {sec: 3.5, loc: locB, rssi: -36, antenna: 9},
			},
			want:       []Crossing{crossing(locA, locB, scanPoint(locB, 9))},
			wantTies:   1,
			wantLeader: locB,
		},
		{
			name: "return",
			reads: []timedRead{
				{sec: 0, loc: locB, rssi: -52}, {sec: 1, loc: locB, rssi: -40},
				{sec: 1.5, loc: locA, rssi: -58}, {sec: 2.5, loc: locA, rssi: -44},
				{sec: 3, loc: locB, rssi: -60}, {sec: 3.5, loc: locA, rssi: -36},
			},
			want:       []Crossing{bToA},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name:       "-10 beats -19",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -19}, {sec: 1, loc: locB, rssi: -10}},
			want:       []Crossing{aToB},
			wantLeader: locB,
		},
		{
			name:       "margin boundary",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -42}},
			want:       []Crossing{aToB},
			wantLeader: locB,
		},
		{
			name:       "under margin",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -43}},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name:       "equal RSSI",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -45}},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name:       "RSSI 0",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -60}, {sec: 1, loc: locB, rssi: 0}},
			wantLeader: locA,
		},
		{
			name: "RSSI 0 is not kept as evidence",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -60}, {sec: 1, loc: locB, rssi: 0},
				{sec: 1.5, loc: locB, rssi: -59},
			},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name:       "positive RSSI",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -60}, {sec: 1, loc: locB, rssi: 5}},
			wantLeader: locA,
		},
		{
			name:       "RSSI 0 never leads a new asset",
			reads:      []timedRead{{sec: 0, loc: locB, rssi: 0}},
			wantLeader: 0,
		},
		{
			name:       "higher inside peak",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -30}, {sec: 3.5, loc: locB, rssi: -35}},
			want:       []Crossing{aToB},
			wantLeader: locB,
		},
		{
			name:       "read exactly one window old still counts",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -30}, {sec: 3, loc: locB, rssi: -35}},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name:       "leader silent",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -40}, {sec: 4, loc: locB, rssi: -70}},
			want:       []Crossing{aToB},
			wantLeader: locB,
		},
		{
			name: "A to B to C",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -50}, {sec: 4, loc: locB, rssi: -50},
				{sec: 8, loc: locC, rssi: -50},
			},
			want: []Crossing{
				aToB,
				crossing(locB, locC, scanPoint(locC, 0)),
			},
			wantLeader: locC,
		},
		{
			name: "same location",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -60, antenna: 1}, {sec: 1, loc: locA, rssi: -30, antenna: 2},
				{sec: 5, loc: locA, rssi: -70, antenna: 1},
			},
			wantLeader: locA,
		},
		{
			name: "antennas on one location count as one",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -40},
				{sec: 1, loc: locB, rssi: -60, antenna: 1}, {sec: 1.5, loc: locB, rssi: -35, antenna: 2},
			},
			want:       []Crossing{crossing(locA, locB, scanPoint(locB, 2))},
			wantTies:   1,
			wantLeader: locB,
		},
		{
			name: "one location only",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -50}, {sec: 1, loc: locA, rssi: -40}, {sec: 600, loc: locA, rssi: -60},
			},
			wantLeader: locA,
		},
		{
			name:  "single read",
			reads: []timedRead{{sec: 0, loc: locA, rssi: -50}}, wantLeader: locA,
		},
		{
			name: "late return",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -50}, {sec: 4, loc: locB, rssi: -50},
				{sec: 604, loc: locA, rssi: -60},
			},
			want:       []Crossing{aToB, bToA},
			wantLeader: locA,
		},
		{
			name: "nil location",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -50}, {sec: 1, loc: noLoc, rssi: -20},
				{sec: 4, loc: locB, rssi: -50},
			},
			want:       []Crossing{aToB},
			wantLeader: locB,
		},
		{
			name: "dedup",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -50}, {sec: 5, loc: locB, rssi: -50},
				{sec: 10, loc: locA, rssi: -50}, {sec: 15, loc: locB, rssi: -50},
			},
			want:       []Crossing{aToB, bToA},
			wantDedup:  1,
			wantLeader: locB,
		},
		{
			name: "dedup expiry",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -50}, {sec: 5, loc: locB, rssi: -50},
				{sec: 10, loc: locA, rssi: -50}, {sec: 71, loc: locB, rssi: -50},
			},
			want:       []Crossing{aToB, bToA, aToB},
			wantLeader: locB,
		},
		{
			name: "a suppressed repeat does not extend the dedup",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -50}, {sec: 5, loc: locB, rssi: -50},
				{sec: 10, loc: locA, rssi: -50}, {sec: 15, loc: locB, rssi: -50},
				{sec: 20, loc: locA, rssi: -50}, {sec: 66, loc: locB, rssi: -50},
			},
			want:       []Crossing{aToB, bToA, aToB},
			wantDedup:  2,
			wantLeader: locB,
		},
		{
			name: "tie counted once",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -45},
				{sec: 0.2, loc: locB, rssi: -44}, {sec: 0.4, loc: locB, rssi: -44},
				{sec: 0.6, loc: locB, rssi: -44}, {sec: 0.8, loc: locB, rssi: -44},
				{sec: 1.0, loc: locB, rssi: -44}, {sec: 1.2, loc: locB, rssi: -44},
			},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name: "leader reads do not break a tie run",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -44},
				{sec: 2, loc: locA, rssi: -45}, {sec: 3, loc: locB, rssi: -44},
				{sec: 4, loc: locA, rssi: -45}, {sec: 5, loc: locB, rssi: -44},
			},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name: "a tie after a quiet window is a new run",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -44},
				{sec: 100, loc: locA, rssi: -45}, {sec: 101, loc: locB, rssi: -44},
			},
			wantTies:   2,
			wantLeader: locA,
		},
		{
			name: "a tie after a crossing is a new run",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -45}, {sec: 0.5, loc: locB, rssi: -44},
				{sec: 1, loc: locB, rssi: -30}, {sec: 1.5, loc: locA, rssi: -31},
			},
			want:       []Crossing{aToB},
			wantTies:   2,
			wantLeader: locB,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTracker(DefaultConfig())
			got := run(tr, tc.reads)

			assert.Equal(t, tc.want, got.crossings)
			assert.Equal(t, tc.wantDedup, got.deduped, "deduped")
			assert.Equal(t, tc.wantTies, got.ties, "ties")
			assert.Equal(t, tc.wantLeader, leaderOf(tr), "leader")
		})
	}
}

func TestTracker_ReadsInOneMessageRunInOrder(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	run(tr, []timedRead{{sec: 0, loc: locA, rssi: -50}})

	res := tr.Evaluate(testOrg, testReader, secs(1), []Read{
		timedRead{loc: locB, rssi: -40}.read(),
		timedRead{loc: locC, rssi: -30}.read(),
	}, nil)
	tr.Apply(res)

	assert.Equal(t, []Crossing{
		crossing(locA, locB, scanPoint(locB, 0)),
		crossing(locB, locC, scanPoint(locC, 0)),
	}, res.Crossings)
	assert.Equal(t, locC, leaderOf(tr))
}

func TestTracker_StateIsPerOrgAssetAndReader(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	run(tr, []timedRead{{sec: 0, loc: locA, rssi: -50}})

	read := timedRead{loc: locB, rssi: -50}.read()
	otherAsset := read
	otherAsset.AssetID = testAsset + 1

	for name, res := range map[string]Result{
		"other org":    tr.Evaluate(testOrg+1, testReader, secs(4), []Read{read}, nil),
		"other reader": tr.Evaluate(testOrg, testReader+1, secs(4), []Read{read}, nil),
		"other asset":  tr.Evaluate(testOrg, testReader, secs(4), []Read{otherAsset}, nil),
	} {
		assert.Empty(t, res.Crossings, name)
	}
}

func TestTracker_ColdStart(t *testing.T) {
	tests := []struct {
		name     string
		seeds    map[int]Seed
		want     []Crossing
		wantCold int
	}{
		{name: "no seed"},
		{name: "unknown seed", seeds: map[int]Seed{testAsset: {}}},
		{
			name:  "seed equal to L",
			seeds: map[int]Seed{testAsset: {Known: true, LocationID: locB, AtReader: true}},
		},
		{
			name:     "seed differs and is at this reader",
			seeds:    map[int]Seed{testAsset: {Known: true, LocationID: locA, AtReader: true}},
			want:     []Crossing{crossing(locA, locB, scanPoint(locB, 0))},
			wantCold: 1,
		},
		{
			name:  "seed differs and is elsewhere",
			seeds: map[int]Seed{testAsset: {Known: true, LocationID: locC}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTracker(DefaultConfig())

			res := tr.Evaluate(testOrg, testReader, secs(0),
				[]Read{timedRead{loc: locB, rssi: -50}.read()}, tc.seeds)
			tr.Apply(res)

			assert.Equal(t, tc.want, res.Crossings)
			assert.Equal(t, tc.wantCold, res.ColdCrossings)
			assert.Equal(t, locB, leaderOf(tr))
		})
	}
}

func TestTracker_ColdStartUsesTheSeedOnlyOnce(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	seeds := map[int]Seed{testAsset: {Known: true, LocationID: locA, AtReader: true}}
	read := timedRead{loc: locB, rssi: -50}.read()

	res := tr.Evaluate(testOrg, testReader, secs(0), []Read{read, read}, seeds)
	tr.Apply(res)
	assert.Len(t, res.Crossings, 1)
	assert.Equal(t, 1, res.ColdCrossings)

	res = tr.Evaluate(testOrg, testReader, secs(1), []Read{read}, seeds)
	assert.Empty(t, res.Crossings)
	assert.Zero(t, res.ColdCrossings)
}

func TestTracker_ColdCrossingStartsTheDedup(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	seeds := map[int]Seed{testAsset: {Known: true, LocationID: locA, AtReader: true}}
	tr.Apply(tr.Evaluate(testOrg, testReader, secs(0), []Read{timedRead{loc: locB, rssi: -50}.read()}, seeds))

	got := run(tr, []timedRead{{sec: 5, loc: locA, rssi: -50}, {sec: 10, loc: locB, rssi: -50}})

	assert.Equal(t, []Crossing{crossing(locB, locA, scanPoint(locA, 0))}, got.crossings)
	assert.Equal(t, 1, got.deduped)
}

func TestTracker_NeedsSeed(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	run(tr, []timedRead{{sec: 0, loc: locA, rssi: -50}})

	assert.Equal(t, []int{testAsset + 1, testAsset + 2},
		tr.NeedsSeed(testOrg, testReader, []int{testAsset + 1, testAsset, testAsset + 2, testAsset + 1}))
	assert.Empty(t, tr.NeedsSeed(testOrg, testReader, []int{testAsset}))
	assert.Equal(t, []int{testAsset}, tr.NeedsSeed(testOrg, testReader+1, []int{testAsset}))
	assert.Equal(t, []int{testAsset}, tr.NeedsSeed(testOrg+1, testReader, []int{testAsset}))
}

func TestTracker_EvaluateDoesNotMutate(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	run(tr, []timedRead{{sec: 0, loc: locA, rssi: -50}, {sec: 1, loc: locB, rssi: -49}})

	reads := []Read{timedRead{loc: locB, rssi: -30}.read()}
	first := tr.Evaluate(testOrg, testReader, secs(5), reads, nil)
	second := tr.Evaluate(testOrg, testReader, secs(5), reads, nil)

	require.Equal(t, []Crossing{crossing(locA, locB, scanPoint(locB, 0))}, first.Crossings)
	assert.Equal(t, first, second)
	assert.Equal(t, locA, leaderOf(tr), "only Apply moves the leader")

	unseen := []Read{{AssetID: testAsset + 1, ScanPointID: 1, LocationID: timedRead{loc: locA}.read().LocationID, RSSI: -50}}
	tr.Evaluate(testOrg, testReader, secs(5), unseen, nil)
	assert.Equal(t, []int{testAsset + 1}, tr.NeedsSeed(testOrg, testReader, []int{testAsset + 1}),
		"evaluating must not create state")

	tr.Apply(first)
	assert.Equal(t, locB, leaderOf(tr))
}

func TestRSSIRules(t *testing.T) {
	assert.True(t, beats(-10, -19, 3))
	assert.False(t, beats(-19, -10, 3))
	assert.True(t, usable(-1))
	assert.False(t, usable(0))
	assert.False(t, usable(5))
}

func TestConfigFromEnv(t *testing.T) {
	tests := []struct {
		name                  string
		window, margin, dedup string
		want                  Config
	}{
		{name: "unset gives the defaults", want: Config{Window: 3 * time.Second, MarginDB: 3, Dedup: time.Minute}},
		{
			name: "well-formed values are honoured", window: "5s", margin: "6", dedup: "2m",
			want: Config{Window: 5 * time.Second, MarginDB: 6, Dedup: 2 * time.Minute},
		},
		{name: "zero margin is honoured", margin: "0", want: Config{Window: 3 * time.Second, Dedup: time.Minute}},
		{name: "unparseable values fall back", window: "soon", margin: "lots", dedup: "60", want: DefaultConfig()},
		{name: "out of range values fall back", window: "0s", margin: "-1", dedup: "-5s", want: DefaultConfig()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CROSSING_WINDOW", tc.window)
			t.Setenv("CROSSING_MARGIN_DB", tc.margin)
			t.Setenv("CROSSING_DEDUP", tc.dedup)

			assert.Equal(t, tc.want, ConfigFromEnv())
		})
	}
}
