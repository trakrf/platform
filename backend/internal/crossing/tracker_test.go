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
	out := Read{AssetID: testAsset, ScanPointID: scanPoint(r.loc, r.antenna), RSSI: r.rssi, At: secs(r.sec)}
	if r.loc != noLoc {
		loc := r.loc
		out.LocationID = &loc
	}
	return out
}

func scanPoint(loc, antenna int) int { return loc*100 + antenna }

// totals sums the per-message results of a run.
type totals struct {
	crossings     []Crossing
	crossedAt     []float64 // the time of the message that produced each crossing
	deduped, ties int
}

func (a *totals) add(r Result) {
	a.crossings = append(a.crossings, r.Crossings...)
	a.deduped += r.Deduped
	a.ties += r.Ties
}

func run(tr *Tracker, reads []timedRead) totals {
	var sum totals
	for _, r := range reads {
		res := tr.Evaluate(testOrg, testReader, []Read{r.read()}, nil)
		tr.Apply(res)
		sum.add(res)
		for range res.Crossings {
			sum.crossedAt = append(sum.crossedAt, r.sec)
		}
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
		wantAt     []float64 // when set, the time of each crossing's message
		wantDedup  int
		wantTies   int
		wantLeader int
	}{
		{
			// The worked example, then one more B read once A's early reads are
			// references: A's peak is -52 against a -60 now, and B's -36 settles it.
			// At 3.5s neither side is the margin ahead, so that read is a tie.
			name: "spec example",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -52}, {sec: 1, loc: locA, rssi: -40},
				{sec: 1.5, loc: locB, rssi: -58}, {sec: 2.5, loc: locB, rssi: -44},
				{sec: 3, loc: locA, rssi: -60}, {sec: 3.5, loc: locB, rssi: -36, antenna: 9},
				{sec: 6.5, loc: locB, rssi: -38},
			},
			want:       []Crossing{crossing(locA, locB, scanPoint(locB, 9))},
			wantAt:     []float64{6.5},
			wantTies:   1,
			wantLeader: locB,
		},
		{
			name: "return",
			reads: []timedRead{
				{sec: 0, loc: locB, rssi: -52}, {sec: 1, loc: locB, rssi: -40},
				{sec: 1.5, loc: locA, rssi: -58}, {sec: 2.5, loc: locA, rssi: -44},
				{sec: 3, loc: locB, rssi: -60}, {sec: 3.5, loc: locA, rssi: -36},
				{sec: 6.5, loc: locA, rssi: -38},
			},
			want:       []Crossing{bToA},
			wantAt:     []float64{6.5},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name: "-10 beats -19",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -19}, {sec: 0.5, loc: locA, rssi: -19},
				{sec: 6, loc: locA, rssi: -25},
				{sec: 6.1, loc: locB, rssi: -10}, {sec: 6.2, loc: locB, rssi: -10},
			},
			want:       []Crossing{aToB},
			wantAt:     []float64{6.2},
			wantLeader: locB,
		},
		{
			// B grows by exactly 3 dB and beats A by exactly 5 dB.
			name: "margin boundary",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -55}, {sec: 0.1, loc: locA, rssi: -55},
				{sec: 0.2, loc: locB, rssi: -60}, {sec: 0.3, loc: locB, rssi: -60},
				{sec: 6, loc: locA, rssi: -62},
				{sec: 6.1, loc: locB, rssi: -57}, {sec: 6.2, loc: locB, rssi: -57},
			},
			want:       []Crossing{aToB},
			wantAt:     []float64{6.2},
			wantLeader: locB,
		},
		{
			// A is 2 dB under its peak: B growing and winning is not enough.
			name: "a fade under the threshold does not cross",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -55}, {sec: 0.1, loc: locA, rssi: -55},
				{sec: 0.2, loc: locB, rssi: -60}, {sec: 0.3, loc: locB, rssi: -60},
				{sec: 6, loc: locA, rssi: -57},
				{sec: 6.1, loc: locB, rssi: -52}, {sec: 6.2, loc: locB, rssi: -52},
			},
			wantLeader: locA,
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
			// A's peak has left the window, so it cannot hide the fade.
			name: "higher inside peak",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -30}, {sec: 0.5, loc: locA, rssi: -30},
				{sec: 6, loc: locA, rssi: -45},
				{sec: 6.1, loc: locB, rssi: -35}, {sec: 6.2, loc: locB, rssi: -35},
			},
			want:       []Crossing{aToB},
			wantAt:     []float64{6.2},
			wantLeader: locB,
		},
		{
			name:       "a read exactly one window old is still heard",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -30}, {sec: 5, loc: locB, rssi: -32}},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			// B grows while A is unheard: the crossing waits for 30s of silence.
			name: "leader silent",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -40},
				{sec: 20, loc: locB, rssi: -70}, {sec: 20.5, loc: locB, rssi: -70},
				{sec: 30, loc: locB, rssi: -70},
			},
			want:       []Crossing{aToB},
			wantAt:     []float64{30},
			wantLeader: locB,
		},
		{
			name: "A to B to C",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -50},
				{sec: 20, loc: locB, rssi: -50}, {sec: 20.5, loc: locB, rssi: -50}, {sec: 30, loc: locB, rssi: -50},
				{sec: 34, loc: locC, rssi: -50}, {sec: 34.5, loc: locC, rssi: -50}, {sec: 60, loc: locC, rssi: -50},
			},
			want: []Crossing{
				aToB,
				crossing(locB, locC, scanPoint(locC, 0)),
			},
			wantAt:     []float64{30, 60},
			wantTies:   1, // C's first read against B, both at -50
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
			// One read on each antenna is two reads at the location: sustained.
			name: "antennas on one location count as one",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -40}, {sec: 0.5, loc: locA, rssi: -40},
				{sec: 6, loc: locA, rssi: -50},
				{sec: 6.1, loc: locB, rssi: -45, antenna: 1}, {sec: 6.2, loc: locB, rssi: -35, antenna: 2},
			},
			want:       []Crossing{crossing(locA, locB, scanPoint(locB, 2))},
			wantAt:     []float64{6.2},
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
			name: "late return on one read",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -50},
				{sec: 20, loc: locB, rssi: -50}, {sec: 20.5, loc: locB, rssi: -50}, {sec: 30, loc: locB, rssi: -50},
				{sec: 630, loc: locA, rssi: -60},
			},
			want:       []Crossing{aToB},
			wantLeader: locB,
		},
		{
			// The absence ends the gap, so the silence clock restarts at 630s and
			// the two A reads cross only once B has been unheard 30s more.
			name: "late return on two reads",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -50},
				{sec: 20, loc: locB, rssi: -50}, {sec: 20.5, loc: locB, rssi: -50}, {sec: 30, loc: locB, rssi: -50},
				{sec: 630, loc: locA, rssi: -60}, {sec: 630.5, loc: locA, rssi: -60}, {sec: 660, loc: locA, rssi: -60},
			},
			want:       []Crossing{aToB, bToA},
			wantAt:     []float64{30, 660},
			wantLeader: locA,
		},
		{
			name: "nil location",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -50}, {sec: 1, loc: noLoc, rssi: -20},
				{sec: 20, loc: locB, rssi: -50}, {sec: 20.5, loc: locB, rssi: -50}, {sec: 30, loc: locB, rssi: -50},
			},
			want:       []Crossing{aToB},
			wantLeader: locB,
		},
		{
			// A to B at 6.3s, back at 12.3s, and the repeat at 18.3s is within the
			// dedup: unrecorded, but the leader still moves.
			name:       "dedup",
			reads:      seq(settle(), swap(6, locA, locB), swap(12, locB, locA), swap(18, locA, locB)),
			want:       []Crossing{aToB, bToA},
			wantAt:     []float64{6.3, 12.3},
			wantDedup:  1,
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
			// B's -44 at 0.2s ties with A's -40. After the swap, two A reads at
			// -20 lift A's window mean back to B's -40: a new run.
			name: "a tie after a crossing is a new run",
			reads: seq([]timedRead{
				{sec: 0, loc: locA, rssi: -40}, {sec: 0.1, loc: locA, rssi: -40},
				{sec: 0.2, loc: locB, rssi: -44}, {sec: 0.3, loc: locB, rssi: -60},
			}, swap(6, locA, locB), []timedRead{
				{sec: 6.4, loc: locA, rssi: -20}, {sec: 6.5, loc: locA, rssi: -20},
			}),
			want:       []Crossing{aToB},
			wantAt:     []float64{6.3},
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
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -49}},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name:       "a read just under the margin above the leader is a tie",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -41}},
			wantTies:   1,
			wantLeader: locA,
		},
		{
			name:       "a read exactly the margin below the leader is not a tie",
			reads:      []timedRead{{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -50}},
			wantLeader: locA,
		},
		{
			name: "leader silent then two weak reads elsewhere cross after the silence",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -30}, {sec: 0.5, loc: locA, rssi: -35}, {sec: 1, loc: locA, rssi: -32},
				{sec: 20, loc: locB, rssi: -80, antenna: 4}, {sec: 20.5, loc: locB, rssi: -80, antenna: 4},
				{sec: 31, loc: locB, rssi: -80, antenna: 4},
			},
			want:       []Crossing{crossing(locA, locB, scanPoint(locB, 4))},
			wantAt:     []float64{31},
			wantLeader: locB,
		},
		{
			name: "the crossing names the strongest read at the new location",
			reads: []timedRead{
				{sec: 0, loc: locA, rssi: -45}, {sec: 0.1, loc: locA, rssi: -45},
				{sec: 6, loc: locA, rssi: -55},
				{sec: 6.1, loc: locB, rssi: -44, antenna: 2}, {sec: 6.2, loc: locB, rssi: -50, antenna: 3},
			},
			want:       []Crossing{crossing(locA, locB, scanPoint(locB, 2))},
			wantAt:     []float64{6.2},
			wantLeader: locB,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTracker(DefaultConfig())
			got := run(tr, tc.reads)

			assert.Equal(t, tc.want, got.crossings)
			if tc.wantAt != nil {
				assert.Equal(t, tc.wantAt, got.crossedAt, "crossing times")
			}
			assert.Equal(t, tc.wantDedup, got.deduped, "deduped")
			assert.Equal(t, tc.wantTies, got.ties, "ties")
			assert.Equal(t, tc.wantLeader, leaderOf(tr), "leader")
		})
	}
}

// One message carrying a whole pass: B grows and A falls silent, then C grows
// and B falls silent. The reads run in stamp order, each at its own time.
func TestTracker_ReadsInOneMessageRunInOrder(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	run(tr, []timedRead{{sec: 0, loc: locA, rssi: -50}})

	res := tr.Evaluate(testOrg, testReader, []Read{
		timedRead{sec: 20, loc: locB, rssi: -50}.read(),
		timedRead{sec: 20.5, loc: locB, rssi: -50}.read(),
		timedRead{sec: 30, loc: locB, rssi: -50}.read(),
		timedRead{sec: 34, loc: locC, rssi: -50}.read(),
		timedRead{sec: 34.5, loc: locC, rssi: -50}.read(),
		timedRead{sec: 60, loc: locC, rssi: -50}.read(),
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
	run(tr, settle())
	run(tr, swap(6, locA, locB)[:2])

	// Two loud B reads cross for this asset at this reader, and nowhere else.
	reads := []Read{timedRead{sec: 6.2, loc: locB, rssi: -40}.read(), timedRead{sec: 6.3, loc: locB, rssi: -40}.read()}
	var another []Read
	for _, r := range reads {
		r.AssetID = testAsset + 1
		another = append(another, r)
	}
	assert.Len(t, tr.Evaluate(testOrg, testReader, reads, nil).Crossings, 1, "same key")
	for name, res := range map[string]Result{
		"other org":    tr.Evaluate(testOrg+1, testReader, reads, nil),
		"other reader": tr.Evaluate(testOrg, testReader+1, reads, nil),
		"other asset":  tr.Evaluate(testOrg, testReader, another, nil),
	} {
		assert.Empty(t, res.Crossings, name)
	}
}

func TestRSSIRules(t *testing.T) {
	assert.True(t, beats(-10, -19, 3))
	assert.False(t, beats(-19, -10, 3))
	assert.True(t, usable(-1))
	assert.False(t, usable(0))
	assert.False(t, usable(5))
}
