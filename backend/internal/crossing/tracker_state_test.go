package crossing

import (
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/storage"
)

// The restart rule: the last known location leads if it is one of this reader's,
// otherwise the first read's location. Neither crosses.
func TestTracker_FirstReadLeader(t *testing.T) {
	tests := []struct {
		name       string
		seeds      map[int]Seed
		wantLeader int
	}{
		{name: "no seed", wantLeader: locB},
		{name: "unknown seed", seeds: map[int]Seed{testAsset: {}}, wantLeader: locB},
		{name: "seed equal to L", seeds: map[int]Seed{testAsset: {Known: true, LocationID: locB, AtReader: true}}, wantLeader: locB},
		{name: "seed differs and is at this reader", seeds: map[int]Seed{testAsset: {Known: true, LocationID: locA, AtReader: true}}, wantLeader: locA},
		{name: "seed differs and is elsewhere", seeds: map[int]Seed{testAsset: {Known: true, LocationID: locC}}, wantLeader: locB},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTracker(DefaultConfig())

			res := tr.Evaluate(testOrg, testReader, []Read{timedRead{loc: locB, rssi: -50}.read()}, tc.seeds)
			tr.Apply(res)

			assert.Empty(t, res.Crossings)
			assert.Equal(t, tc.wantLeader, leaderOf(tr))
		})
	}
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

// snapshot deep-copies everything the tracker holds.
func snapshot(tr *Tracker) (map[stateKey]*assetState, map[readerKey]map[int]bool) {
	states := make(map[stateKey]*assetState, len(tr.states))
	for k, s := range tr.states {
		states[k] = s.clone()
	}
	waiting := make(map[readerKey]map[int]bool, len(tr.waiting))
	for k, ids := range tr.waiting {
		waiting[k] = maps.Clone(ids)
	}
	return states, waiting
}

func TestTracker_EvaluateDoesNotMutate(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	run(tr, []timedRead{
		{sec: 0, loc: locA, rssi: -50}, {sec: 1, loc: locB, rssi: -50}, {sec: 1.5, loc: locB, rssi: -49},
	})
	states, waiting := snapshot(tr)
	require.Equal(t, map[int]bool{testAsset: true}, waiting[readerKey{org: testOrg, reader: testReader}])

	for name, reads := range map[string][]Read{
		"a read of its own":       {timedRead{sec: 31, loc: locB, rssi: -30}.read()},
		"another tag's message":   {otherTag(31)},
		"a message of both":       {otherTag(31), timedRead{sec: 31, loc: locC, rssi: -30}.read()},
		"a read with no RSSI":     {timedRead{sec: 31, loc: locB, rssi: 0}.read()},
		"a read with no location": {timedRead{sec: 31, loc: noLoc, rssi: -30}.read()},
	} {
		t.Run(name, func(t *testing.T) {
			first := tr.Evaluate(testOrg, testReader, reads, nil)
			second := tr.Evaluate(testOrg, testReader, reads, nil)

			assert.Contains(t, first.Crossings, crossing(locA, locB, scanPoint(locB, 0)), "the silence check fires")
			assert.Equal(t, first, second)
			assert.Equal(t, states, tr.states)
			assert.Equal(t, waiting, tr.waiting)
		})
	}

	unseen := []Read{{AssetID: testAsset + 1, ScanPointID: 1, LocationID: timedRead{loc: locA}.read().LocationID, RSSI: -50, At: secs(5)}}
	tr.Evaluate(testOrg, testReader, unseen, nil)
	assert.Equal(t, []int{testAsset + 1}, tr.NeedsSeed(testOrg, testReader, []int{testAsset + 1}),
		"evaluating must not create state")

	tr.Apply(tr.Evaluate(testOrg, testReader, []Read{otherTag(31)}, nil))
	assert.Equal(t, locB, leaderOf(tr))
	assert.Empty(t, tr.waiting[readerKey{org: testOrg, reader: testReader}], "a crossing uses up the evidence")
}

// A discarded Result must leave warm state exactly as it was, read history
// included. The reference tracker saw the same committed history.
func TestTracker_DiscardedResultLeavesWarmStateUnchanged(t *testing.T) {
	history := []timedRead{
		{sec: 0, loc: locA, rssi: -40}, {sec: 1, loc: locA, rssi: -45},
		{sec: 1.5, loc: locB, rssi: -44, antenna: 2}, {sec: 2, loc: locB, rssi: -60},
	}
	tr, ref := NewTracker(DefaultConfig()), NewTracker(DefaultConfig())
	run(tr, history)
	run(ref, history)

	// At 32s A has been silent long enough for the silence check; the reads also
	// age the history and add a location. None of it may reach the live state.
	res := tr.Evaluate(testOrg, testReader, []Read{
		timedRead{sec: 32, loc: locC, rssi: -50}.read(),
		timedRead{sec: 32.5, loc: locC, rssi: -50}.read(),
		timedRead{sec: 33, loc: locA, rssi: -30}.read(),
	}, nil)
	require.NotEmpty(t, res.Crossings)

	assert.Equal(t, ref.states, tr.states)
	assert.Equal(t, ref.waiting, tr.waiting)
}

const (
	locD = 4
	// otherAsset is another tag at the same reader.
	otherAsset = testAsset + 1
)

func otherTag(sec float64) Read {
	r := timedRead{sec: sec, loc: locC, rssi: -60}.read()
	r.AssetID = otherAsset
	return r
}

func seedAt(l int) map[int]Seed {
	return map[int]Seed{testAsset: {Known: true, LocationID: l, AtReader: true}}
}

// play delivers each read as its own message, with another tag's message every
// tick seconds in between (0 for none). The asset starts led by leader.
func play(tr *Tracker, leader int, reads []timedRead, tick, end float64) totals {
	var sum totals
	seeds := seedAt(leader)
	deliver := func(sec float64, r Read) {
		res := tr.Evaluate(testOrg, testReader, []Read{r}, seeds)
		tr.Apply(res)
		sum.add(res)
		for range res.Crossings {
			sum.crossedAt = append(sum.crossedAt, sec)
		}
	}
	next := tick
	for _, r := range reads {
		for tick > 0 && next < r.sec {
			deliver(next, otherTag(next))
			next += tick
		}
		deliver(r.sec, r.read())
	}
	for tick > 0 && next <= end {
		deliver(next, otherTag(next))
		next += tick
	}
	return sum
}

func routesOf(cs []Crossing) [][2]int {
	var out [][2]int
	for _, c := range cs {
		if c.AssetID == testAsset {
			out = append(out, [2]int{c.From, c.To})
		}
	}
	return out
}

func TestTracker_OneReadDoesNotCross(t *testing.T) {
	for name, sec := range map[string]float64{"soon after": 1, "after 40s": 40, "after a long absence": 600} {
		t.Run(name, func(t *testing.T) {
			tr := NewTracker(DefaultConfig())
			got := play(tr, locA, []timedRead{{sec: 0, loc: locA, rssi: -40}, {sec: sec, loc: locB, rssi: -30}}, 5, sec+120)
			assert.Empty(t, routesOf(got.crossings))
			assert.Equal(t, locA, leaderOf(tr))
		})
	}
	t.Run("when B has references and A then falls silent", func(t *testing.T) {
		// Two early B reads give B references; A is heard throughout, so they are
		// no evidence. One loud B read as A stops is still one read.
		reads := bySec(append(every(0.5, 20.5, locA, -40),
			timedRead{sec: 0.25, loc: locB, rssi: -60}, timedRead{sec: 0.75, loc: locB, rssi: -60},
			timedRead{sec: 20.2, loc: locB, rssi: -30}))
		tr := NewTracker(DefaultConfig())
		got := play(tr, locA, reads, 5, 120)
		assert.Empty(t, routesOf(got.crossings))
		assert.Equal(t, locA, leaderOf(tr))
	})
}

// The absence ends the gap: the two A reads start a fresh silence clock, and the
// crossing comes on the first message once B has been unheard 30s more.
func TestTracker_ReturnAfterAbsenceCrosses(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	got := play(tr, locB, []timedRead{
		{sec: 0, loc: locB, rssi: -50}, {sec: 600, loc: locA, rssi: -60}, {sec: 600.5, loc: locA, rssi: -60},
	}, 5, 700)
	assert.Equal(t, [][2]int{{locB, locA}}, routesOf(got.crossings))
	assert.Equal(t, []float64{630}, got.crossedAt)
	assert.Equal(t, locA, leaderOf(tr))
}

// A parked boat heard on both sides, a two-minute outage, then reads again with
// B's first. The gap resets the evidence, so the order back does not matter.
func TestTracker_OutageDoesNotFlip(t *testing.T) {
	levels := swapAt(0, nil, map[int]int{locA: -50, locB: -52})
	reads := append(cycles(0, 60, levels, locA, locB), cycles(180, 240, levels, locB, locA)...)
	tr := NewTracker(DefaultConfig())
	got := play(tr, locA, reads, 5, 300)
	assert.Empty(t, routesOf(got.crossings))
	assert.Equal(t, locA, leaderOf(tr))
}

func TestTracker_RestartKeepsLastSide(t *testing.T) {
	for name, order := range readOrders {
		t.Run(name, func(t *testing.T) {
			tr := NewTracker(DefaultConfig())
			got := play(tr, locA, cycles(0, 60, swapAt(0, nil, map[int]int{locA: -50, locB: -52}), order...), 5, 120)
			assert.Empty(t, routesOf(got.crossings))
			assert.Equal(t, locA, leaderOf(tr))
		})
	}
}

func TestTracker_RestartMidPassCrossesAfterSilence(t *testing.T) {
	var reads []timedRead
	for sec := 0.0; sec < 40; sec += 0.5 {
		reads = append(reads, timedRead{sec: sec, loc: locB, rssi: -70 + int(sec)})
	}
	tr := NewTracker(DefaultConfig())
	got := play(tr, locA, reads, 0, 0)
	assert.Equal(t, [][2]int{{locA, locB}}, routesOf(got.crossings))
	assert.Equal(t, []float64{30}, got.crossedAt)
	assert.Equal(t, locB, leaderOf(tr))
}

// A silence crossing fired by another tag's message and then dropped fires again
// on the retry, and once applied, never again.
func TestTracker_SilenceCrossingSurvivesARollback(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	play(tr, locA, []timedRead{
		{sec: 0, loc: locA, rssi: -45}, {sec: 1, loc: locB, rssi: -50}, {sec: 1.5, loc: locB, rssi: -50},
	}, 0, 0)
	want := []Crossing{crossing(locA, locB, scanPoint(locB, 0))}

	dropped := tr.Evaluate(testOrg, testReader, []Read{otherTag(31)}, nil)
	assert.Equal(t, want, dropped.Crossings)
	assert.Equal(t, locA, leaderOf(tr), "only Apply moves the leader")

	retry := tr.Evaluate(testOrg, testReader, []Read{otherTag(32)}, nil)
	assert.Equal(t, want, retry.Crossings)
	tr.Apply(retry)
	assert.Equal(t, locB, leaderOf(tr))

	assert.Empty(t, tr.Evaluate(testOrg, testReader, []Read{otherTag(33)}, nil).Crossings)
}

func TestTracker_ReaderTimeWindow(t *testing.T) {
	t.Run("reads stamped apart in one message age out of each other", func(t *testing.T) {
		for gap, want := range map[float64]int{0.5: 1, 6: 0} {
			tr := NewTracker(DefaultConfig())
			run(tr, []timedRead{{sec: 0, loc: locA, rssi: -40}, {sec: 0.5, loc: locA, rssi: -40}, {sec: 6, loc: locA, rssi: -50}})
			res := tr.Evaluate(testOrg, testReader, []Read{
				timedRead{sec: 6.1, loc: locB, rssi: -40}.read(),
				timedRead{sec: 6.1 + gap, loc: locB, rssi: -40}.read(),
			}, nil)
			assert.Len(t, res.Crossings, want, "gap %v", gap)
		}
	})
	t.Run("a 30s delivery delay with correct stamps changes nothing", func(t *testing.T) {
		assert.Equal(t, [][2]int{{locA, locB}}, stageRoutes(t, 0, true))
		assert.Equal(t, [][2]int{{locA, locB}}, stageRoutes(t, 30*time.Second, true))
	})
}

func TestTracker_MissingReaderTimeUsesServerTime(t *testing.T) {
	reads, _ := orderedReads([]storage.ResolvedRead{
		rd(testAsset, 201, locA, -50, time.Time{}),
		rd(testAsset, 202, locB, -50, recvAt.Add(-time.Second)),
	}, recvAt)
	require.Len(t, reads, 2)
	assert.Equal(t, recvAt.Add(-time.Second), reads[0].At, "a reader timestamp is kept")
	assert.Equal(t, recvAt, reads[1].At, "a missing one takes the receive time")

	assert.Equal(t, [][2]int{{locA, locB}}, stageRoutes(t, 0, false), "aged by receive time, the swap still shows")
}

// Reader time is trusted up to 5 minutes behind the receive time, and never ahead.
func TestTracker_ReaderTimeTolerance(t *testing.T) {
	for off, want := range map[time.Duration]time.Duration{
		0:                            0,
		-5 * time.Minute:             -5 * time.Minute,
		-5*time.Minute - time.Second: 0,
		time.Second:                  0,
		5 * time.Minute:              0,
	} {
		got, _ := orderedReads([]storage.ResolvedRead{rd(testAsset, 201, locA, -50, recvAt.Add(off))}, recvAt)
		assert.Equal(t, recvAt.Add(want), got[0].At, "offset %v", off)
	}
}

// Another tag's read stamped minutes ahead runs the silence check at the receive
// time, not at that stamp: the boat's leader has been silent 10s, not 4 minutes.
func TestTracker_OneReadAheadDoesNotFireOthersSilence(t *testing.T) {
	s, _, rec := seeded()
	commitAt := func(at time.Time, reads ...storage.ResolvedRead) {
		commit, err := detect(t, s, input(at, reads...))
		require.NoError(t, err)
		commit(true)
	}
	commitAt(recvAt, rd(testAsset, 201, locA, -45, time.Time{}))
	commitAt(recvAt.Add(time.Second), twice(testAsset, 202, locB, -50, time.Time{})...)
	require.Empty(t, rec.got)

	ahead := recvAt.Add(10 * time.Second)
	commitAt(ahead, rd(99, 201, locA, -60, ahead.Add(4*time.Minute)))
	assert.Empty(t, rec.got, "10s of silence is not 30")

	commitAt(recvAt.Add(31*time.Second), rd(99, 201, locA, -60, time.Time{}))
	require.Len(t, rec.got, 1)
	assert.Equal(t, [2]int{locA, locB}, routeOf(rec.got[0]))
}

// A read stamped minutes ahead takes the receive time, so the asset's clock stays
// in the present and the reads that follow still age and judge normally.
func TestTracker_ReadAheadDoesNotFreeze(t *testing.T) {
	s, _, rec := seeded()
	for _, m := range []struct {
		at    time.Duration
		reads []storage.ResolvedRead
	}{
		{0, []storage.ResolvedRead{rd(testAsset, 201, locA, -40, recvAt.Add(4*time.Minute))}},
		{500 * time.Millisecond, []storage.ResolvedRead{rd(testAsset, 201, locA, -40, time.Time{})}},
		{6 * time.Second, []storage.ResolvedRead{rd(testAsset, 201, locA, -50, time.Time{})}},
		{6200 * time.Millisecond, []storage.ResolvedRead{
			rd(testAsset, 202, locB, -40, recvAt.Add(6100*time.Millisecond)), rd(testAsset, 202, locB, -40, time.Time{}),
		}},
	} {
		commit, err := detect(t, s, input(recvAt.Add(m.at), m.reads...))
		require.NoError(t, err)
		commit(true)
	}
	require.Len(t, rec.got, 1)
	assert.Equal(t, [2]int{locA, locB}, routeOf(rec.got[0]))
	assert.Equal(t, recvAt.Add(6200*time.Millisecond), rec.got[0].DetectedAt)
}

// A read stamped earlier than the asset's clock joins the history by its stamp and
// leaves the clock alone; one stamped more than History earlier is dropped.
func TestTracker_ClockBackDoesNotGrowWindow(t *testing.T) {
	tr := NewTracker(DefaultConfig())
	run(tr, []timedRead{{sec: 0, loc: locA, rssi: -40}, {sec: 20, loc: locA, rssi: -40}})
	got := run(tr, []timedRead{{sec: 10, loc: locB, rssi: -30}})
	assert.Empty(t, got.crossings)

	s := tr.states[stateKey{org: testOrg, asset: testAsset, reader: testReader}]
	assert.Equal(t, secs(20), s.clock)
	require.Len(t, s.reads[locB], 1)
	assert.Equal(t, secs(10), s.reads[locB][0].at)
	assert.Equal(t, secs(10), s.heard[locB].at)
	assert.Zero(t, s.level(locB, tr.cfg).heard, "ten seconds old, the read is a reference, not heard")

	run(tr, []timedRead{{sec: -50, loc: locB, rssi: -30}})
	assert.Len(t, s.reads[locB], 1, "older than History before the clock: dropped")
	assert.Equal(t, secs(20), s.clock)
}

func TestConfigFromEnv(t *testing.T) {
	env := func(window, history, minReads, margin, fade, grow, silence, gap, dedup string) map[string]string {
		return map[string]string{
			"CROSSING_WINDOW": window, "CROSSING_HISTORY": history, "CROSSING_MIN_READS": minReads,
			"CROSSING_MARGIN_DB": margin, "CROSSING_FADE_DB": fade, "CROSSING_GROW_DB": grow,
			"CROSSING_SILENCE": silence, "CROSSING_GAP": gap, "CROSSING_DEDUP": dedup,
		}
	}
	tests := []struct {
		name string
		env  map[string]string
		want Config
	}{
		{name: "unset gives the defaults", env: env("", "", "", "", "", "", "", "", ""), want: DefaultConfig()},
		{
			name: "well-formed values are honoured", env: env("6s", "90s", "3", "6", "4", "2", "20s", "45s", "2m"),
			want: Config{Window: 6 * time.Second, History: 90 * time.Second, MinReads: 3, MarginDB: 6, FadeDB: 4, GrowDB: 2,
				Silence: 20 * time.Second, Gap: 45 * time.Second, Dedup: 2 * time.Minute},
		},
		{
			name: "zero dB values are honoured", env: env("", "", "", "0", "0", "0", "", "", ""),
			want: func() Config { c := DefaultConfig(); c.MarginDB, c.FadeDB, c.GrowDB = 0, 0, 0; return c }(),
		},
		{name: "unparseable values fall back", env: env("soon", "x", "two", "lots", "y", "z", "w", "v", "60"), want: DefaultConfig()},
		{name: "out of range values fall back", env: env("0s", "-1s", "0", "-1", "-1", "-1", "0s", "0s", "-5s"), want: DefaultConfig()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			assert.Equal(t, tc.want, ConfigFromEnv())
		})
	}
}
