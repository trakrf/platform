package report

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToPublicCurrentLocationItem_LiveAsset(t *testing.T) {
	loc := "BAY-3"
	locID := 42
	in := CurrentLocationItem{
		AssetID:             7,
		AssetExternalKey:    "FORK-007",
		LocationID:          &locID,
		LocationExternalKey: &loc,
		LastSeen:            time.Date(2026, 4, 25, 18, 33, 0, 0, time.UTC),
		AssetDeletedAt:      nil,
	}

	got := ToPublicCurrentLocationItem(in)

	require.NotNil(t, got.AssetID)
	assert.Equal(t, 7, *got.AssetID)
	require.NotNil(t, got.AssetExternalKey)
	assert.Equal(t, "FORK-007", *got.AssetExternalKey)
	require.NotNil(t, got.LocationID)
	assert.Equal(t, 42, *got.LocationID)
	require.NotNil(t, got.LocationExternalKey)
	assert.Equal(t, "BAY-3", *got.LocationExternalKey)
	assert.Nil(t, got.AssetDeletedAt)

	// Live row must omit asset_deleted_at from JSON entirely.
	data, err := json.Marshal(got)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	_, present := parsed["asset_deleted_at"]
	assert.False(t, present, "asset_deleted_at must be omitted when nil")
}

// AC11: open dwell period (most recent scan, no later scan) must serialize
// duration_seconds as explicit null rather than being omitted, so generated
// clients see a consistent field shape across closed and open periods.
func TestToPublicAssetHistoryItem_OpenPeriodEmitsNullDuration(t *testing.T) {
	loc := "BAY-3"
	locID := 42
	in := AssetHistoryItem{
		Timestamp:           time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC),
		LocationID:          &locID,
		LocationExternalKey: &loc,
		DurationSeconds:     nil,
	}

	got := ToPublicAssetHistoryItem(in)

	data, err := json.Marshal(got)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	raw, present := parsed["duration_seconds"]
	assert.True(t, present, "duration_seconds must be present in JSON, not omitted")
	assert.Nil(t, raw, "duration_seconds must serialize as null on open period")
}

func TestToPublicAssetHistoryItem_ClosedPeriodEmitsDuration(t *testing.T) {
	loc := "BAY-3"
	locID := 42
	dur := 3600
	in := AssetHistoryItem{
		Timestamp:           time.Date(2026, 4, 27, 10, 0, 0, 0, time.UTC),
		LocationID:          &locID,
		LocationExternalKey: &loc,
		DurationSeconds:     &dur,
	}

	got := ToPublicAssetHistoryItem(in)

	data, err := json.Marshal(got)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	assert.EqualValues(t, 3600, parsed["duration_seconds"])
}

// TRA-547 §2.3: PublicCurrentLocationItem.asset_deleted_at is omitted for live assets.
func TestPublicCurrentLocationItem_AssetDeletedAtAbsentWhenNil(t *testing.T) {
	id := 1
	key := "A1"
	locID := 9
	locKey := "L1"
	it := PublicCurrentLocationItem{
		AssetID:             &id,
		AssetExternalKey:    &key,
		LocationID:          &locID,
		LocationExternalKey: &locKey,
	}
	data, err := json.Marshal(it)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	_, present := parsed["asset_deleted_at"]
	assert.False(t, present, "asset_deleted_at must be omitted when nil per TRA-547 §2.3")
}

func TestToPublicCurrentLocationItem_DeletedAsset(t *testing.T) {
	loc := "BAY-3"
	locID := 42
	deletedAt := time.Date(2026, 4, 20, 14, 0, 0, 0, time.UTC)
	in := CurrentLocationItem{
		AssetID:             7,
		AssetExternalKey:    "FORK-007",
		LocationID:          &locID,
		LocationExternalKey: &loc,
		LastSeen:            time.Date(2026, 4, 25, 18, 33, 0, 0, time.UTC),
		AssetDeletedAt:      &deletedAt,
	}

	got := ToPublicCurrentLocationItem(in)

	require.NotNil(t, got.AssetDeletedAt)
	assert.Equal(t, deletedAt, *got.AssetDeletedAt)

	data, err := json.Marshal(got)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "2026-04-20T14:00:00Z", parsed["asset_deleted_at"])
}
