package crossing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
		{
			name:       "a read clearly weaker than the leader is not a tie",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -60}},
			wantLeader: locA,
		},
		{
			name:       "a read just under the margin below the leader is a tie",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -47}},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name:       "a read just under the margin above the leader is a tie",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -43}},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name:       "a read exactly the margin below the leader is not a tie",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -48}},
			wantLeader: locA,
		},
		{
			name: "leader reads aged out then one weak read elsewhere is a crossing",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -30}, {sec: 0.5, loc: locA, rssi: -35}, {sec: 1, loc: locA, rssi: -32},
				{sec: 5, loc: locB, rssi: -80, antenna: 4},
			},
			want:       []Crossing{crossing(locA, locB, scanPoint(locB, 4))},
			wantLeader: locB,
		},
		{
			name: "the crossing names the strongest read at the new location",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -45},
				{sec: 1, loc: locB, rssi: -44, antenna: 2}, {sec: 4, loc: locB, rssi: -50, antenna: 3},
			},
			want:       []Crossing{crossing(locA, locB, scanPoint(locB, 2))},
			wantTies:   1,
			wantLeader: locB,
		},
		{
			name: "the crossing names the strongest read while the leader is still recent",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -40}, {sec: 2, loc: locA, rssi: -50},
				{sec: 2.5, loc: locB, rssi: -42, antenna: 2}, {sec: 3.5, loc: locB, rssi: -48, antenna: 3},
			},
			want:       []Crossing{crossing(locA, locB, scanPoint(locB, 2))},
			wantTies:   1,
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
