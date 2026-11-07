package assetevent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/storage"
)

type fakeMoveStore struct {
	prev   map[int]storage.PreviousLocation
	err    error
	before time.Time
	calls  int
}

func (f *fakeMoveStore) PreviousAssetLocationsTx(_ context.Context, _ pgx.Tx, _ int, _ []int, before time.Time) (map[int]storage.PreviousLocation, error) {
	f.calls++
	f.before = before
	return f.prev, f.err
}

type fakeChangeRecorder struct {
	got  []notificationevent.LocationChange
	err  error
	skip bool // report the change as already recorded
}

func (f *fakeChangeRecorder) RecordChange(_ context.Context, _ pgx.Tx, c notificationevent.LocationChange) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.got = append(f.got, c)
	return !f.skip, nil
}

func moveInput(reads ...storage.ResolvedRead) storage.DetectInput {
	return storage.DetectInput{
		OrgID:      7,
		ReceivedAt: time.Date(2026, 11, 7, 10, 5, 42, 0, time.UTC),
		Reads:      reads,
	}
}

func lookupErrors() float64 {
	var m dto.Metric
	if err := metricLookupErrors.Write(&m); err != nil {
		panic(err)
	}
	return m.GetCounter().GetValue()
}

func TestMoveStage_RecordsMove(t *testing.T) {
	store := &fakeMoveStore{prev: map[int]storage.PreviousLocation{1: {LocationID: intp(10)}}}
	rec := &fakeChangeRecorder{}
	s := NewMoveStage(store, rec)
	assert.Equal(t, "reader_move", s.Name())

	p, err := s.Detect(context.Background(), nil, moveInput(
		storage.ResolvedRead{AssetID: 1, ScanPointID: 4, LocationID: intp(20), Stored: true}))
	require.NoError(t, err)
	assert.Nil(t, p.OnCommit)
	assert.Nil(t, p.OnRollback)
	require.Len(t, rec.got, 1)
	c := rec.got[0]
	assert.Equal(t, notificationevent.OriginReader, c.Origin)
	assert.Equal(t, 7, c.OrgID)
	assert.Equal(t, 1, c.AssetID)
	require.NotNil(t, c.FromLocationID)
	assert.Equal(t, 10, *c.FromLocationID)
	assert.Equal(t, 20, c.ToLocationID)
	require.NotNil(t, c.ScanPointID)
	assert.Equal(t, 4, *c.ScanPointID)
	want := time.Date(2026, 11, 7, 10, 5, 0, 0, time.UTC)
	assert.Equal(t, want, c.DetectedAt)
	assert.Equal(t, want, store.before)
}

func TestMoveStage_SkipsUnchangedLocation(t *testing.T) {
	store := &fakeMoveStore{prev: map[int]storage.PreviousLocation{1: {LocationID: intp(20)}}}
	rec := &fakeChangeRecorder{}
	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(
		storage.ResolvedRead{AssetID: 1, ScanPointID: 4, LocationID: intp(20), Stored: true}))
	require.NoError(t, err)
	assert.Empty(t, rec.got)
}

func TestMoveStage_SkipsReadsNotStored(t *testing.T) {
	store := &fakeMoveStore{}
	rec := &fakeChangeRecorder{}
	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(
		storage.ResolvedRead{AssetID: 1, ScanPointID: 4, LocationID: intp(20), Stored: false}))
	require.NoError(t, err)
	assert.Empty(t, rec.got)
	assert.Zero(t, store.calls)
}

func TestMoveStage_SkipsReadsWithNoLocation(t *testing.T) {
	store := &fakeMoveStore{}
	rec := &fakeChangeRecorder{}
	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(
		storage.ResolvedRead{AssetID: 1, ScanPointID: 4, Stored: true}))
	require.NoError(t, err)
	assert.Empty(t, rec.got)
	assert.Zero(t, store.calls)
}

func TestMoveStage_FirstSightingHasNoFrom(t *testing.T) {
	store := &fakeMoveStore{prev: map[int]storage.PreviousLocation{}}
	rec := &fakeChangeRecorder{}
	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(
		storage.ResolvedRead{AssetID: 1, ScanPointID: 4, LocationID: intp(20), Stored: true}))
	require.NoError(t, err)
	require.Len(t, rec.got, 1)
	assert.Nil(t, rec.got[0].FromLocationID)
}

func TestMoveStage_OneMovePerAssetPerMessage(t *testing.T) {
	store := &fakeMoveStore{prev: map[int]storage.PreviousLocation{}}
	rec := &fakeChangeRecorder{}
	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(
		storage.ResolvedRead{AssetID: 1, ScanPointID: 4, LocationID: intp(20), Stored: true},
		storage.ResolvedRead{AssetID: 1, ScanPointID: 5, LocationID: intp(21), Stored: true}))
	require.NoError(t, err)
	require.Len(t, rec.got, 1)
	assert.Equal(t, 20, rec.got[0].ToLocationID)
}

func TestMoveStage_LookupErrorReturnsError(t *testing.T) {
	store := &fakeMoveStore{err: errors.New("db down")}
	rec := &fakeChangeRecorder{}
	before := lookupErrors()
	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(
		storage.ResolvedRead{AssetID: 1, ScanPointID: 4, LocationID: intp(20), Stored: true}))
	require.Error(t, err)
	assert.Empty(t, rec.got)
	assert.Equal(t, before+1, lookupErrors())
}

func TestMoveStage_AlreadyRecordedIsNotAnError(t *testing.T) {
	store := &fakeMoveStore{prev: map[int]storage.PreviousLocation{}}
	rec := &fakeChangeRecorder{skip: true}
	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(
		storage.ResolvedRead{AssetID: 1, ScanPointID: 4, LocationID: intp(20), Stored: true}))
	require.NoError(t, err)
}

func TestMoveStage_RecordErrorReturnsError(t *testing.T) {
	store := &fakeMoveStore{prev: map[int]storage.PreviousLocation{}}
	rec := &fakeChangeRecorder{err: errors.New("insert failed")}
	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(
		storage.ResolvedRead{AssetID: 1, ScanPointID: 4, LocationID: intp(20), Stored: true}))
	require.ErrorContains(t, err, "insert failed")
}

func TestMoveStage_PreviousSightingWithoutLocationRecordsNilFrom(t *testing.T) {
	store := &fakeMoveStore{prev: map[int]storage.PreviousLocation{1: {LocationID: nil}}}
	rec := &fakeChangeRecorder{}
	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(
		storage.ResolvedRead{AssetID: 1, ScanPointID: 4, LocationID: intp(20), Stored: true}))
	require.NoError(t, err)
	require.Len(t, rec.got, 1)
	assert.Nil(t, rec.got[0].FromLocationID)
}

func TestMoveStage_NotStoredThenStoredReadForOneAsset(t *testing.T) {
	store := &fakeMoveStore{prev: map[int]storage.PreviousLocation{}}
	rec := &fakeChangeRecorder{}
	_, err := NewMoveStage(store, rec).Detect(context.Background(), nil, moveInput(
		storage.ResolvedRead{AssetID: 1, ScanPointID: 4, LocationID: intp(20), Stored: false},
		storage.ResolvedRead{AssetID: 1, ScanPointID: 5, LocationID: intp(21), Stored: true}))
	require.NoError(t, err)
	require.Len(t, rec.got, 1)
	assert.Equal(t, 21, rec.got[0].ToLocationID)
	assert.Equal(t, 5, *rec.got[0].ScanPointID)
}
