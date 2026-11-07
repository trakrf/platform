package crossing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cold marks a crossing as one that came from a seed.
func cold(c Crossing) Crossing { c.Cold = true; return c }

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
			want:     []Crossing{cold(crossing(locA, locB, scanPoint(locB, 0)))},
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

// A discarded Result must leave warm state exactly as it was, recent reads
// included. The reference tracker saw the same committed history.
func TestTracker_DiscardedResultLeavesWarmStateUnchanged(t *testing.T) {
	history := []timedRead{
		{sec: 0, loc: locA, rssi: -40}, {sec: 1, loc: locA, rssi: -45},
		{sec: 1.5, loc: locB, rssi: -44, antenna: 2}, {sec: 2, loc: locB, rssi: -60},
	}
	tr, ref := NewTracker(DefaultConfig()), NewTracker(DefaultConfig())
	run(tr, history)
	run(ref, history)

	// At 6s every earlier read has aged out of the window; the new reads add a
	// location and cross. None of it may reach the live state.
	res := tr.Evaluate(testOrg, testReader, secs(6), []Read{
		timedRead{loc: locC, rssi: -50}.read(),
		timedRead{loc: locA, rssi: -30}.read(),
	}, nil)
	require.NotEmpty(t, res.Crossings)

	assert.Equal(t, ref.states, tr.states)
}
