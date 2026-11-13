package crossing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/storage"
)

// scanTx stands in for the scan transaction: the stage must hand it to both lookups.
type scanTx struct{ pgx.Tx }

type fakeStore struct {
	prev       map[int]storage.PreviousLocation
	locs       []int
	prevErr    error
	locErr     error
	prevCalls  int
	locCalls   int
	lastBefore time.Time
	lastIDs    []int
	txs        []pgx.Tx
}

func (f *fakeStore) PreviousAssetLocationsTx(_ context.Context, tx pgx.Tx, _ int, ids []int, before time.Time) (map[int]storage.PreviousLocation, error) {
	f.prevCalls++
	f.txs = append(f.txs, tx)
	f.lastBefore, f.lastIDs = before, ids
	return f.prev, f.prevErr
}

func (f *fakeStore) ScanDeviceLocationIDsTx(_ context.Context, tx pgx.Tx, _, _ int) ([]int, error) {
	f.locCalls++
	f.txs = append(f.txs, tx)
	return f.locs, f.locErr
}

type fakeRecorder struct {
	got    []notificationevent.LocationChange
	replay bool
	err    error
}

func (f *fakeRecorder) RecordChange(_ context.Context, _ pgx.Tx, c notificationevent.LocationChange) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.got = append(f.got, c)
	return !f.replay, nil
}

var recvAt = time.Date(2026, 11, 7, 12, 0, 30, 500, time.UTC)

func loc(id int) *int { return &id }

func rd(asset, sp, location, rssi int, ts time.Time) storage.ResolvedRead {
	return storage.ResolvedRead{AssetID: asset, ScanPointID: sp, LocationID: loc(location), RSSI: rssi, ReaderTimestamp: ts}
}

func input(at time.Time, reads ...storage.ResolvedRead) storage.DetectInput {
	return storage.DetectInput{OrgID: testOrg, ScanDeviceID: testReader, ReceivedAt: at, Reads: reads}
}

// seeded returns a stage whose asset 42 was last seen at location A.
func seeded() (*Stage, *fakeStore, *fakeRecorder) {
	st := &fakeStore{
		prev: map[int]storage.PreviousLocation{testAsset: {LocationID: loc(locA)}},
		locs: []int{locA, locB, locC},
	}
	rec := &fakeRecorder{}
	return NewStage(NewTracker(DefaultConfig()), st, rec), st, rec
}

func detect(t *testing.T, s *Stage, in storage.DetectInput) (func(bool), error) {
	t.Helper()
	p, err := s.Detect(context.Background(), &scanTx{}, in)
	return p.OnCommit, err
}

// warm commits a boat whose A was last heard 31s before at and whose B has grown
// since, with a B read 15s before at to keep the gap closed: the next message
// crosses by silence. The recorder starts empty.
func warm(t *testing.T) (*Stage, *fakeStore, *fakeRecorder) {
	t.Helper()
	return warmAt(t, recvAt)
}

func warmAt(t *testing.T, at time.Time) (*Stage, *fakeStore, *fakeRecorder) {
	t.Helper()
	s, st, rec := seeded()
	for _, m := range []struct {
		before time.Duration
		reads  []storage.ResolvedRead
	}{
		{31 * time.Second, []storage.ResolvedRead{rd(testAsset, 201, locA, -45, time.Time{})}},
		{30500 * time.Millisecond, twice(testAsset, 202, locB, -50, time.Time{})},
		{15 * time.Second, []storage.ResolvedRead{rd(testAsset, 202, locB, -50, time.Time{})}},
	} {
		commit, err := detect(t, s, input(at.Add(-m.before), m.reads...))
		require.NoError(t, err)
		commit(true)
	}
	require.Empty(t, rec.got)
	require.Equal(t, locA, leaderOf(s.tracker))
	return s, st, rec
}

// twice is the same read two times: enough to show growth at a location with no
// references.
func twice(asset, sp, location, rssi int, ts time.Time) []storage.ResolvedRead {
	return []storage.ResolvedRead{rd(asset, sp, location, rssi, ts), rd(asset, sp, location, rssi, ts)}
}

func routeOf(c notificationevent.LocationChange) [2]int {
	return [2]int{*c.FromLocationID, c.ToLocationID}
}

func routes(cs []notificationevent.LocationChange) [][2]int {
	var out [][2]int
	for _, c := range cs {
		out = append(out, routeOf(c))
	}
	return out
}

func TestStage_Name(t *testing.T) {
	assert.Equal(t, "crossing", NewStage(nil, nil, nil).Name())
}

func TestStage_RecordsCrossing(t *testing.T) {
	s, _, rec := warm(t)
	commit, err := detect(t, s, input(recvAt, rd(testAsset, 202, locB, -50, time.Time{})))
	require.NoError(t, err)
	commit(true)

	require.Len(t, rec.got, 1)
	c := rec.got[0]
	assert.Equal(t, notificationevent.OriginReaderCrossing, c.Origin)
	assert.Equal(t, testOrg, c.OrgID)
	assert.Equal(t, testAsset, c.AssetID)
	require.NotNil(t, c.FromLocationID)
	assert.Equal(t, locA, *c.FromLocationID)
	assert.Equal(t, locB, c.ToLocationID)
	require.NotNil(t, c.ScanPointID)
	assert.Equal(t, 202, *c.ScanPointID)
	assert.Equal(t, recvAt, c.DetectedAt)
}

func TestStage_LookupsRunOnTheScanTransaction(t *testing.T) {
	s, st, _ := seeded()
	tx := &scanTx{}
	_, err := s.Detect(context.Background(), tx, input(recvAt, rd(testAsset, 202, locB, -50, time.Time{})))
	require.NoError(t, err)
	require.Len(t, st.txs, 2)
	for _, got := range st.txs {
		assert.Same(t, tx, got)
	}
}

// twoHops is a message, received 31s after at, whose reader stamps span a warm
// boat's crossing to B at at, C growing just after, and C again once B has been
// silent 30s.
func twoHops(at time.Time) []storage.ResolvedRead {
	return []storage.ResolvedRead{
		rd(testAsset, 202, locB, -40, at),
		rd(testAsset, 203, locC, -20, at.Add(500*time.Millisecond)),
		rd(testAsset, 203, locC, -20, at.Add(time.Second)),
		rd(testAsset, 203, locC, -20, at.Add(30500*time.Millisecond)),
	}
}

func TestStage_TwoCrossingsInOneMessageHaveDistinctTimes(t *testing.T) {
	s, _, rec := warm(t)
	_, err := detect(t, s, input(recvAt.Add(31*time.Second), twoHops(recvAt)...))
	require.NoError(t, err)
	require.Equal(t, [][2]int{{locA, locB}, {locB, locC}}, routes(rec.got))
	assert.Equal(t, time.Microsecond, rec.got[1].DetectedAt.Sub(rec.got[0].DetectedAt))
}

// A receive time on a scan bucket floor, at the database's microsecond precision,
// is also the reader move's detected_at, so crossings move on and stay distinct.
func TestStage_CrossingsAvoidTheBucketFloor(t *testing.T) {
	floor := recvAt.Truncate(storage.ScanGranularity).Add(storage.ScanGranularity)
	stamped := floor.Add(-31 * time.Second)
	for name, received := range map[string]time.Time{
		"exactly on it":           floor,
		"under a microsecond off": floor.Add(999 * time.Nanosecond),
	} {
		t.Run(name, func(t *testing.T) {
			s, _, rec := warmAt(t, stamped)
			_, err := detect(t, s, input(received, twoHops(stamped)...))
			require.NoError(t, err)
			require.Len(t, rec.got, 2)
			assert.Equal(t, floor.Add(time.Microsecond), rec.got[0].DetectedAt)
			assert.Equal(t, floor.Add(2*time.Microsecond), rec.got[1].DetectedAt)
		})
	}
	assert.Equal(t, floor.Add(time.Microsecond), crossingBase(floor.Add(time.Microsecond)), "a microsecond on needs no nudge")
}

// Reader order is the input to direction: however they arrive, B's read comes
// first, so A to B then B to C.
func TestStage_SortsByReaderTimestamp(t *testing.T) {
	hops := twoHops(recvAt)
	for name, reads := range map[string][]storage.ResolvedRead{
		"in order":     hops,
		"out of order": {hops[3], hops[1], hops[2], hops[0]},
	} {
		t.Run(name, func(t *testing.T) {
			s, _, rec := warm(t)
			_, err := detect(t, s, input(recvAt.Add(31*time.Second), reads...))
			require.NoError(t, err)
			assert.Equal(t, [][2]int{{locA, locB}, {locB, locC}}, routes(rec.got))
		})
	}
}

// With no reader clock every read takes the receive time and the arrival order
// stands. A has faded; B and C both grow in one message. Whichever comes first
// takes the crossing, and the other, with the new leader heard alongside it and
// no references to show a fade, does not.
func TestStage_MissingReaderTimestampsKeepArrivalOrder(t *testing.T) {
	b := twice(testAsset, 202, locB, -40, time.Time{})
	c := twice(testAsset, 203, locC, -20, time.Time{})
	for name, tc := range map[string]struct {
		reads []storage.ResolvedRead
		want  [][2]int
	}{
		"B first": {append(b, c...), [][2]int{{locA, locB}}},
		"C first": {append(c, b...), [][2]int{{locA, locC}}},
	} {
		t.Run(name, func(t *testing.T) {
			s, _, rec := seeded()
			for _, m := range []struct {
				at    time.Duration
				reads []storage.ResolvedRead
			}{
				{0, twice(testAsset, 201, locA, -40, time.Time{})},
				{6 * time.Second, []storage.ResolvedRead{rd(testAsset, 201, locA, -50, time.Time{})}},
				{6200 * time.Millisecond, tc.reads},
			} {
				commit, err := detect(t, s, input(recvAt.Add(m.at), m.reads...))
				require.NoError(t, err)
				commit(true)
			}
			assert.Equal(t, tc.want, routes(rec.got))
		})
	}
}

func TestStage_StateAppliedOnlyWhenRowsKept(t *testing.T) {
	s, _, rec := warm(t)
	in := input(recvAt, rd(testAsset, 202, locB, -50, time.Time{}))

	commit, err := detect(t, s, in)
	require.NoError(t, err)
	commit(false)
	commit, err = detect(t, s, in)
	require.NoError(t, err)
	require.Len(t, rec.got, 2, "state was not applied, so the crossing is detected again")

	commit(true)
	_, err = detect(t, s, in)
	require.NoError(t, err)
	assert.Len(t, rec.got, 2, "state was applied, so the crossing is not detected again")
}

// A discarded message must leave warm state, read history included, as a stage
// that never saw it. The reference stage saw the same committed messages.
func TestStage_DiscardedMessageLeavesWarmStateUnchanged(t *testing.T) {
	committed := []storage.DetectInput{
		input(recvAt, append(twice(testAsset, 202, locB, -44, time.Time{}), rd(testAsset, 201, locA, -40, time.Time{}))...),
		input(recvAt.Add(time.Second), rd(testAsset, 201, locA, -45, time.Time{})),
	}
	s, _, rec := seeded()
	ref, _, _ := seeded()
	for _, in := range committed {
		commit, err := detect(t, s, in)
		require.NoError(t, err)
		commit(true)
		commit, err = detect(t, ref, in)
		require.NoError(t, err)
		commit(true)
	}

	// Thirty seconds on A is silent, and this message crosses to C. Dropped, it
	// must leave nothing behind.
	commit, err := detect(t, s, input(recvAt.Add(31*time.Second), twice(testAsset, 203, locC, -50, time.Time{})...))
	require.NoError(t, err)
	require.NotEmpty(t, rec.got)
	commit(false)

	assert.Equal(t, ref.tracker.states, s.tracker.states)
	assert.Equal(t, ref.tracker.waiting, s.tracker.waiting)
}

// A restart with a yard of parked tags. The first hundred are heard on both
// antennas, last seen on either side; the rest on one antenna, last seen there.
// Nothing crosses on the first message, or on messages over the next minute.
func TestStage_RestartWithParkedTags(t *testing.T) {
	st := &fakeStore{prev: map[int]storage.PreviousLocation{}, locs: []int{locA, locB}}
	var reads []storage.ResolvedRead
	for i := 1; i <= 200; i++ {
		l := locA + i%2
		st.prev[i] = storage.PreviousLocation{LocationID: loc(l)}
		reads = append(reads, rd(i, 100+l, l, -50, time.Time{}))
		if i <= 100 {
			other := locA + locB - l
			reads = append(reads, rd(i, 100+other, other, -52+i%5, time.Time{}))
		}
	}
	rec := &fakeRecorder{}
	s := NewStage(NewTracker(DefaultConfig()), st, rec)

	for sec := 0; sec < 60; sec++ {
		commit, err := detect(t, s, input(recvAt.Add(time.Duration(sec)*time.Second), reads...))
		require.NoError(t, err)
		commit(true)
	}

	assert.Empty(t, rec.got)
	assert.Equal(t, 1, st.prevCalls)
	assert.Equal(t, 1, st.locCalls)
	assert.Equal(t, recvAt.Truncate(storage.ScanGranularity), st.lastBefore)
	assert.Len(t, st.lastIDs, 200)
}

// Building a Seed: the last known location only counts when it is one of this
// reader's own, and a sighting with no location says nothing.
func TestStage_SeedsFromTheLastKnownLocation(t *testing.T) {
	tests := []struct {
		name       string
		prev       map[int]storage.PreviousLocation
		wantLeader int
	}{
		{name: "last seen at another of this reader's locations", prev: map[int]storage.PreviousLocation{testAsset: {LocationID: loc(locA)}}, wantLeader: locA},
		{name: "last seen elsewhere", prev: map[int]storage.PreviousLocation{testAsset: {LocationID: loc(locC)}}, wantLeader: locB},
		{name: "last sighting had no location", prev: map[int]storage.PreviousLocation{testAsset: {}}, wantLeader: locB},
		{name: "never seen", prev: map[int]storage.PreviousLocation{}, wantLeader: locB},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &fakeRecorder{}
			s := NewStage(NewTracker(DefaultConfig()), &fakeStore{prev: tc.prev, locs: []int{locA, locB}}, rec)
			commit, err := detect(t, s, input(recvAt, rd(testAsset, 202, locB, -50, time.Time{})))
			require.NoError(t, err)
			commit(true)

			assert.Empty(t, rec.got, "a restart never crosses by itself")
			assert.Equal(t, tc.wantLeader, leaderOf(s.tracker))
		})
	}
}

func TestStage_SkipsLookupsWithoutNewAssets(t *testing.T) {
	s, st, _ := warm(t)
	require.Equal(t, 1, st.prevCalls)

	_, err := detect(t, s, input(recvAt, rd(testAsset, 201, locA, -50, time.Time{})))
	require.NoError(t, err)
	assert.Equal(t, 1, st.prevCalls, "a known asset needs no lookup")
	assert.Equal(t, 1, st.locCalls)
}

func TestStage_LookupErrorReturnsError(t *testing.T) {
	for name, set := range map[string]func(*fakeStore){
		"previous locations": func(f *fakeStore) { f.prevErr = errors.New("db down") },
		"reader locations":   func(f *fakeStore) { f.locErr = errors.New("db down") },
	} {
		t.Run(name, func(t *testing.T) {
			s, st, rec := seeded()
			set(st)
			p, err := s.Detect(context.Background(), &scanTx{}, input(recvAt, rd(testAsset, 202, locB, -50, time.Time{})))
			require.Error(t, err)
			assert.Nil(t, p.OnCommit)
			assert.Empty(t, rec.got)
		})
	}
}

func TestStage_RecordErrorReturnsError(t *testing.T) {
	s, _, rec := warm(t)
	rec.err = errors.New("insert failed")
	_, err := detect(t, s, input(recvAt, rd(testAsset, 202, locB, -50, time.Time{})))
	require.Error(t, err)
}

// A silence crossing fired by another tag's message and then dropped: the retry
// records it once, and once kept it does not fire again.
func TestStage_SilenceCrossingOnAnotherTagsMessageRollsBack(t *testing.T) {
	s, _, rec := seeded()
	boat := func(at time.Time, reads ...storage.ResolvedRead) {
		commit, err := detect(t, s, input(at, reads...))
		require.NoError(t, err)
		commit(true)
	}
	boat(recvAt, rd(testAsset, 201, locA, -45, time.Time{}))
	boat(recvAt.Add(time.Second), twice(testAsset, 202, locB, -50, time.Time{})...)
	require.Empty(t, rec.got)

	other := func(at time.Time) storage.DetectInput { return input(at, rd(99, 201, locA, -60, time.Time{})) }
	dropped, err := detect(t, s, other(recvAt.Add(31*time.Second)))
	require.NoError(t, err)
	require.Len(t, rec.got, 1)
	dropped(false)

	kept, err := detect(t, s, other(recvAt.Add(32*time.Second)))
	require.NoError(t, err)
	require.Len(t, rec.got, 2, "dropped, so it fires again")
	assert.Equal(t, testAsset, rec.got[1].AssetID)
	assert.Equal(t, [2]int{locA, locB}, routeOf(rec.got[1]))
	kept(true)

	_, err = detect(t, s, other(recvAt.Add(33*time.Second)))
	require.NoError(t, err)
	assert.Len(t, rec.got, 2, "kept, so it does not fire again")
}

// stageRoutes plays a swap through the stage, one read per message, each delivered
// after delay, and returns the recorded routes.
func stageRoutes(t *testing.T, delay time.Duration, stamped bool) [][2]int {
	t.Helper()
	s, _, rec := seeded()
	for _, r := range cycles(0, 60, swapAt(30,
		map[int]int{locA: -40, locB: -60}, map[int]int{locA: -55, locB: -42}), locA, locB) {
		at := recvAt.Add(time.Duration(r.sec * float64(time.Second)))
		stamp := time.Time{}
		if stamped {
			stamp = at
		}
		commit, err := detect(t, s, input(at.Add(delay), rd(testAsset, scanPoint(r.loc, 0), r.loc, r.rssi, stamp)))
		require.NoError(t, err)
		commit(true)
	}
	return routes(rec.got)
}
