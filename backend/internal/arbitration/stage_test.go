package arbitration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/storage"
)

type scanTx struct{ pgx.Tx }

type fakeSeedStore struct {
	prev   map[int]storage.PreviousLocation
	err    error
	calls  int
	before time.Time
	ids    []int
	tx     pgx.Tx
}

func (f *fakeSeedStore) PreviousAssetLocationsTx(_ context.Context, tx pgx.Tx, _ int, ids []int, before time.Time) (map[int]storage.PreviousLocation, error) {
	f.calls++
	f.tx, f.ids, f.before = tx, ids, before
	return f.prev, f.err
}

func resolved(sp, loc, rssi int) storage.ResolvedRead {
	return storage.ResolvedRead{AssetID: asset, ScanPointID: sp, LocationID: ptr(loc), RSSI: rssi}
}

var recv = time.Date(2026, 11, 27, 12, 0, 30, 250, time.UTC)

func TestStageSeedsFromTheStoredLocationIncludingThisMinute(t *testing.T) {
	store := &fakeSeedStore{prev: map[int]storage.PreviousLocation{asset: {LocationID: ptr(locA)}}}
	st := NewStage(New(testConfig()), store)
	tx := scanTx{}

	got, err := st.Begin().Arbitrate(context.Background(), tx, storage.ArbitrateInput{
		OrgID: org, ReceivedAt: recv, Reads: []storage.ResolvedRead{resolved(spB, locB, -80)},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, store.calls)
	assert.Equal(t, tx, store.tx, "lookup runs on the scan transaction")
	assert.Equal(t, []int{asset}, store.ids)
	assert.Equal(t, recv.Truncate(time.Minute).Add(time.Minute), store.before, "this minute's row counts")
	assert.Equal(t, storage.ArbitratedLocation{LocationID: locA, Hold: true}, got[asset],
		"seeded location not heard yet: hold")
}

func TestStageAssignsAndAppliesOnlyOnCommit(t *testing.T) {
	store := &fakeSeedStore{}
	st := NewStage(New(testConfig()), store)
	in := storage.ArbitrateInput{OrgID: org, ReceivedAt: recv, Reads: []storage.ResolvedRead{resolved(spA, locA, -50)}}

	run := st.Begin()
	got, err := run.Arbitrate(context.Background(), scanTx{}, in)
	require.NoError(t, err)
	assert.Equal(t, storage.ArbitratedLocation{LocationID: locA, ScanPointID: spA}, got[asset])
	// Rolled back: never committed, so the next scan seeds again.
	_, err = st.Begin().Arbitrate(context.Background(), scanTx{}, in)
	require.NoError(t, err)
	assert.Equal(t, 2, store.calls)

	run = st.Begin()
	_, err = run.Arbitrate(context.Background(), scanTx{}, in)
	require.NoError(t, err)
	run.Committed()
	_, err = st.Begin().Arbitrate(context.Background(), scanTx{}, in)
	require.NoError(t, err)
	assert.Equal(t, 3, store.calls, "committed state needs no seed")
}

func TestStageUsesTheServerReceiveTime(t *testing.T) {
	st := NewStage(New(testConfig()), &fakeSeedStore{})
	r := resolved(spA, locA, -50)
	r.ReaderTimestamp = recv.Add(-time.Hour) // a gateway clock is not trusted across gateways
	run := st.Begin()
	_, err := run.Arbitrate(context.Background(), scanTx{}, storage.ArbitrateInput{OrgID: org, ReceivedAt: recv, Reads: []storage.ResolvedRead{r}})
	require.NoError(t, err)
	run.Committed()

	// B is far stronger from here, but only 4s of server time pass: no switch.
	run = st.Begin()
	got, err := run.Arbitrate(context.Background(), scanTx{}, storage.ArbitrateInput{OrgID: org, ReceivedAt: recv.Add(time.Second), Reads: []storage.ResolvedRead{resolved(spB, locB, -20)}})
	require.NoError(t, err)
	run.Committed()
	run = st.Begin()
	got, err = run.Arbitrate(context.Background(), scanTx{}, storage.ArbitrateInput{OrgID: org, ReceivedAt: recv.Add(5 * time.Second), Reads: []storage.ResolvedRead{resolved(spB, locB, -20)}})
	require.NoError(t, err)
	assert.Equal(t, locA, got[asset].LocationID)
}

func TestStageLeavesUndecidedAssetsOut(t *testing.T) {
	st := NewStage(New(testConfig()), &fakeSeedStore{})
	got, err := st.Begin().Arbitrate(context.Background(), scanTx{}, storage.ArbitrateInput{
		OrgID: org, ReceivedAt: recv, Reads: []storage.ResolvedRead{resolved(spA, locA, 0)},
	})
	require.NoError(t, err)
	assert.Empty(t, got, "no usable RSSI and nothing stored: stored as observed")
}

func TestStageSeedLookupErrorFailsTheScan(t *testing.T) {
	st := NewStage(New(testConfig()), &fakeSeedStore{err: errors.New("db down")})
	_, err := st.Begin().Arbitrate(context.Background(), scanTx{}, storage.ArbitrateInput{
		OrgID: org, ReceivedAt: recv, Reads: []storage.ResolvedRead{resolved(spA, locA, -50)},
	})
	require.Error(t, err)
}
