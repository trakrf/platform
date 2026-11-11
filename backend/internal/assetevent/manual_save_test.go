package assetevent

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

type fakeManualStore struct {
	prev   map[int]storage.PreviousLocation
	err    error
	calls  int
	ids    []int
	before time.Time
}

func (f *fakeManualStore) PreviousAssetLocationsTx(_ context.Context, _ pgx.Tx, _ int, assetIDs []int, before time.Time) (map[int]storage.PreviousLocation, error) {
	f.calls++
	f.ids = assetIDs
	f.before = before
	return f.prev, f.err
}

var savedMinute = time.Date(2026, 11, 10, 9, 30, 0, 0, time.UTC)

func savedInventory(inserted []int, overridden map[int]*int) storage.SavedInventory {
	return storage.SavedInventory{
		OrgID:            7,
		LocationID:       20,
		Timestamp:        savedMinute,
		InsertedAssetIDs: inserted,
		OverriddenFrom:   overridden,
	}
}

func TestManualSave_RecordsMoveOnLocationChange(t *testing.T) {
	store := &fakeManualStore{prev: map[int]storage.PreviousLocation{1: {LocationID: intp(10)}}}
	rec := &fakeChangeRecorder{}

	err := NewManualSave(store, rec).Record(context.Background(), nil, savedInventory([]int{1}, nil))
	require.NoError(t, err)

	require.Len(t, rec.got, 1)
	c := rec.got[0]
	require.NoError(t, c.Validate())
	assert.Equal(t, notificationevent.OriginManualSave, c.Origin)
	assert.Equal(t, 7, c.OrgID)
	assert.Equal(t, 1, c.AssetID)
	require.NotNil(t, c.FromLocationID)
	assert.Equal(t, 10, *c.FromLocationID)
	assert.Equal(t, 20, c.ToLocationID)
	assert.Nil(t, c.ScanPointID)
	assert.Equal(t, savedMinute, c.DetectedAt)
	assert.Equal(t, savedMinute, store.before, "the lookup is strictly before the saved minute")
}

func TestManualSave_FirstSightingIsQuiet(t *testing.T) {
	store := &fakeManualStore{prev: map[int]storage.PreviousLocation{}}
	rec := &fakeChangeRecorder{}

	err := NewManualSave(store, rec).Record(context.Background(), nil, savedInventory([]int{1}, nil))
	require.NoError(t, err)
	assert.Empty(t, rec.got)
}

func TestManualSave_SameLocationIsQuiet(t *testing.T) {
	store := &fakeManualStore{prev: map[int]storage.PreviousLocation{1: {LocationID: intp(20)}}}
	rec := &fakeChangeRecorder{}

	err := NewManualSave(store, rec).Record(context.Background(), nil, savedInventory([]int{1}, nil))
	require.NoError(t, err)
	assert.Empty(t, rec.got)
}

func TestManualSave_PreviousSightingWithoutLocationRecordsFromNil(t *testing.T) {
	store := &fakeManualStore{prev: map[int]storage.PreviousLocation{1: {LocationID: nil}}}
	rec := &fakeChangeRecorder{}

	err := NewManualSave(store, rec).Record(context.Background(), nil, savedInventory([]int{1}, nil))
	require.NoError(t, err)
	require.Len(t, rec.got, 1)
	require.NoError(t, rec.got[0].Validate())
	assert.Nil(t, rec.got[0].FromLocationID)
	assert.Equal(t, notificationevent.OriginManualSave, rec.got[0].Origin)
}

func TestManualSave_OverrideRecordsCorrection(t *testing.T) {
	store := &fakeManualStore{}
	rec := &fakeChangeRecorder{}
	beforeCall := time.Now()

	err := NewManualSave(store, rec).Record(context.Background(), nil, savedInventory(nil, map[int]*int{3: intp(11)}))
	require.NoError(t, err)

	assert.Zero(t, store.calls, "an override carries its own from-location; no lookup")
	require.Len(t, rec.got, 1)
	c := rec.got[0]
	require.NoError(t, c.Validate())
	assert.Equal(t, notificationevent.OriginManualOverride, c.Origin)
	assert.Equal(t, 3, c.AssetID)
	require.NotNil(t, c.FromLocationID)
	assert.Equal(t, 11, *c.FromLocationID)
	assert.Equal(t, 20, c.ToLocationID)
	assert.Nil(t, c.ScanPointID)
	assert.True(t, c.DetectedAt.After(savedMinute), "a correction sorts after the row it corrected")
	assert.False(t, c.DetectedAt.Before(beforeCall), "wall clock, not the saved minute")
	assert.NotEqual(t, c.DetectedAt, c.DetectedAt.Truncate(storage.ScanGranularity), "not truncated")
}

func TestManualSave_OverrideFromNilRecordsFromNil(t *testing.T) {
	store := &fakeManualStore{}
	rec := &fakeChangeRecorder{}

	err := NewManualSave(store, rec).Record(context.Background(), nil, savedInventory(nil, map[int]*int{3: nil}))
	require.NoError(t, err)
	require.Len(t, rec.got, 1)
	require.NoError(t, rec.got[0].Validate())
	assert.Nil(t, rec.got[0].FromLocationID)
	assert.Equal(t, notificationevent.OriginManualOverride, rec.got[0].Origin)
}

func TestManualSave_OneLookupPerSave(t *testing.T) {
	store := &fakeManualStore{prev: map[int]storage.PreviousLocation{
		1: {LocationID: intp(10)},
		2: {LocationID: intp(20)},
	}}
	rec := &fakeChangeRecorder{}

	err := NewManualSave(store, rec).Record(context.Background(), nil, savedInventory([]int{1, 2, 3}, nil))
	require.NoError(t, err)

	assert.Equal(t, 1, store.calls)
	assert.Equal(t, []int{1, 2, 3}, store.ids)
	require.Len(t, rec.got, 1, "1 moved, 2 stayed, 3 is a first sighting")
	assert.Equal(t, 1, rec.got[0].AssetID)
}

func TestManualSave_NothingSavedSkipsLookup(t *testing.T) {
	store := &fakeManualStore{}
	rec := &fakeChangeRecorder{}

	err := NewManualSave(store, rec).Record(context.Background(), nil, savedInventory(nil, nil))
	require.NoError(t, err)
	assert.Zero(t, store.calls)
	assert.Empty(t, rec.got)
}

func TestManualSave_LookupErrorReturnsError(t *testing.T) {
	store := &fakeManualStore{err: errors.New("db down")}
	rec := &fakeChangeRecorder{}

	err := NewManualSave(store, rec).Record(context.Background(), nil, savedInventory([]int{1}, nil))
	require.Error(t, err)
	assert.Empty(t, rec.got)
}

func TestManualSave_RecordErrorReturnsError(t *testing.T) {
	store := &fakeManualStore{prev: map[int]storage.PreviousLocation{1: {LocationID: intp(10)}}}
	rec := &fakeChangeRecorder{err: errors.New("insert failed")}

	err := NewManualSave(store, rec).Record(context.Background(), nil, savedInventory([]int{1}, nil))
	require.Error(t, err)
}

func TestManualSave_AlreadyRecordedIsNotAnError(t *testing.T) {
	store := &fakeManualStore{prev: map[int]storage.PreviousLocation{1: {LocationID: intp(10)}}}
	rec := &fakeChangeRecorder{skip: true}

	err := NewManualSave(store, rec).Record(context.Background(), nil, savedInventory([]int{1}, nil))
	require.NoError(t, err)
}
