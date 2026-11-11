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

// warm commits a message so the stage tracks asset 42 with leader A and the
// recorder starts empty.
func warm(t *testing.T) (*Stage, *fakeStore, *fakeRecorder) {
	t.Helper()
	s, st, rec := seeded()
	commit, err := detect(t, s, input(recvAt.Add(-time.Hour), rd(testAsset, 201, locA, -50, time.Time{})))
	require.NoError(t, err)
	commit(true)
	require.Empty(t, rec.got)
	return s, st, rec
}

func routeOf(c notificationevent.LocationChange) [2]int {
	return [2]int{*c.FromLocationID, c.ToLocationID}
}

func TestStage_Name(t *testing.T) {
	assert.Equal(t, "crossing", NewStage(nil, nil, nil).Name())
}

func TestStage_RecordsCrossing(t *testing.T) {
	s, _, rec := seeded()
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

func TestStage_TwoCrossingsInOneMessageHaveDistinctTimes(t *testing.T) {
	s, _, rec := warm(t)
	t0 := recvAt.Add(-time.Second)
	_, err := detect(t, s, input(recvAt,
		rd(testAsset, 202, locB, -40, t0),
		rd(testAsset, 203, locC, -20, t0.Add(time.Second)),
	))
	require.NoError(t, err)
	require.Len(t, rec.got, 2)
	assert.Equal(t, time.Microsecond, rec.got[1].DetectedAt.Sub(rec.got[0].DetectedAt))
}

// Reader order is the input to direction. The leader is A at -50: read in order,
// B (-40) then C (-20) is two crossings; read C first, B never beats C.
func TestStage_SortsByReaderTimestamp(t *testing.T) {
	t0 := recvAt.Add(-time.Minute)
	for name, reads := range map[string][]storage.ResolvedRead{
		"in order":     {rd(testAsset, 202, locB, -40, t0), rd(testAsset, 203, locC, -20, t0.Add(time.Millisecond))},
		"out of order": {rd(testAsset, 203, locC, -20, t0.Add(time.Millisecond)), rd(testAsset, 202, locB, -40, t0)},
	} {
		t.Run(name, func(t *testing.T) {
			s, _, rec := warm(t)
			_, err := detect(t, s, input(recvAt, reads...))
			require.NoError(t, err)
			require.Len(t, rec.got, 2)
			assert.Equal(t, [2]int{locA, locB}, routeOf(rec.got[0]))
			assert.Equal(t, [2]int{locB, locC}, routeOf(rec.got[1]))
		})
	}
}

// With no reader clock the arrival order stands: B then C is two crossings, and
// a reversed order would give one crossing A to C.
func TestStage_MissingReaderTimestampsKeepArrivalOrder(t *testing.T) {
	s, _, rec := warm(t)
	_, err := detect(t, s, input(recvAt,
		rd(testAsset, 202, locB, -40, time.Time{}),
		rd(testAsset, 203, locC, -20, time.Time{}),
	))
	require.NoError(t, err)
	require.Len(t, rec.got, 2)
	assert.Equal(t, [2]int{locA, locB}, routeOf(rec.got[0]))
	assert.Equal(t, [2]int{locB, locC}, routeOf(rec.got[1]))
}

func TestStage_StateAppliedOnlyWhenRowsKept(t *testing.T) {
	s, _, rec := seeded()
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

// A discarded message must leave warm state, recent reads included, as a stage
// that never saw it. The reference stage saw the same committed messages.
func TestStage_DiscardedMessageLeavesWarmStateUnchanged(t *testing.T) {
	committed := []storage.DetectInput{
		input(recvAt, rd(testAsset, 201, locA, -40, time.Time{}), rd(testAsset, 202, locB, -44, time.Time{})),
		input(recvAt.Add(time.Second), rd(testAsset, 201, locA, -45, time.Time{})),
	}
	s, _, _ := seeded()
	ref, _, _ := seeded()
	for _, in := range committed {
		commit, err := detect(t, s, in)
		require.NoError(t, err)
		commit(true)
		commit, err = detect(t, ref, in)
		require.NoError(t, err)
		commit(true)
	}

	// Ten seconds on every earlier read has aged out; this one also crosses.
	commit, err := detect(t, s, input(recvAt.Add(10*time.Second), rd(testAsset, 203, locC, -50, time.Time{})))
	require.NoError(t, err)
	commit(false)

	assert.Equal(t, ref.tracker.states, s.tracker.states)
}

func TestStage_RestartWithParkedTags(t *testing.T) {
	st := &fakeStore{prev: map[int]storage.PreviousLocation{}, locs: []int{locA, locB}}
	var reads []storage.ResolvedRead
	for i := 1; i <= 200; i++ {
		l := locA + i%2
		st.prev[i] = storage.PreviousLocation{LocationID: loc(l)}
		reads = append(reads, rd(i, 100+l, l, -50, time.Time{}))
	}
	rec := &fakeRecorder{}
	s := NewStage(NewTracker(DefaultConfig()), st, rec)

	_, err := detect(t, s, input(recvAt, reads...))
	require.NoError(t, err)
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
		name      string
		prev      map[int]storage.PreviousLocation
		locs      []int
		wantRoute *[2]int
	}{
		{name: "last seen at another of this reader's locations", prev: map[int]storage.PreviousLocation{testAsset: {LocationID: loc(locA)}},
			locs: []int{locA, locB}, wantRoute: &[2]int{locA, locB}},
		{name: "last seen elsewhere", prev: map[int]storage.PreviousLocation{testAsset: {LocationID: loc(locC)}},
			locs: []int{locA, locB}},
		{name: "last sighting had no location", prev: map[int]storage.PreviousLocation{testAsset: {}},
			locs: []int{locA, locB}},
		{name: "never seen", prev: map[int]storage.PreviousLocation{}, locs: []int{locA, locB}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := &fakeRecorder{}
			s := NewStage(NewTracker(DefaultConfig()), &fakeStore{prev: tc.prev, locs: tc.locs}, rec)
			commit, err := detect(t, s, input(recvAt, rd(testAsset, 202, locB, -50, time.Time{})))
			require.NoError(t, err)
			commit(true)

			if tc.wantRoute == nil {
				assert.Empty(t, rec.got)
			} else {
				require.Len(t, rec.got, 1)
				assert.Equal(t, *tc.wantRoute, routeOf(rec.got[0]))
			}
			assert.Equal(t, locB, leaderOf(s.tracker), "the read's location leads either way")
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
	s, _, rec := seeded()
	rec.err = errors.New("insert failed")
	_, err := detect(t, s, input(recvAt, rd(testAsset, 202, locB, -50, time.Time{})))
	require.Error(t, err)
}
