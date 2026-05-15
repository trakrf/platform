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

	// TRA-732 R4: asset_id and asset_external_key are non-nullable.
	assert.Equal(t, 7, got.AssetID)
	assert.Equal(t, "FORK-007", got.AssetExternalKey)
	require.NotNil(t, got.LocationID)
	assert.Equal(t, 42, *got.LocationID)
	require.NotNil(t, got.LocationExternalKey)
	assert.Equal(t, "BAY-3", *got.LocationExternalKey)
	assert.Nil(t, got.AssetDeletedAt)

	// TRA-610 / BB18 §1.10: live row must always emit asset_deleted_at
	// as null (not omit). Generated clients see a stable shape across
	// live and deleted assets.
	data, err := json.Marshal(got)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	raw, present := parsed["asset_deleted_at"]
	assert.True(t, present, "asset_deleted_at must always be present (TRA-610)")
	assert.Nil(t, raw, "asset_deleted_at must be JSON null for live assets (TRA-610)")

	// TRA-717 / BB34 F2: asset-locations rows emit `asset_last_seen`
	// (qualifier-prefix matches asset_deleted_at convention).
	_, alsOk := parsed["asset_last_seen"]
	assert.True(t, alsOk, "asset_last_seen must be present on asset-locations rows")
	_, oldOk := parsed["last_seen"]
	assert.False(t, oldOk, "old `last_seen` field must not be emitted after TRA-717 rename")
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

	// TRA-717 / BB34 F2: history rows emit `event_observed_at`
	// (qualifier-prefix harmonization across same-primitive endpoints).
	_, evOk := parsed["event_observed_at"]
	assert.True(t, evOk, "event_observed_at must be present on history rows")
	_, oldOk := parsed["timestamp"]
	assert.False(t, oldOk, "old `timestamp` field must not be emitted after TRA-717 rename")
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

// TRA-610 / BB18 §1.10: PublicCurrentLocationItem.asset_deleted_at is
// always emitted (null for live assets). Supersedes the TRA-547 §2.3
// omit-when-nil behavior.
func TestPublicCurrentLocationItem_AssetDeletedAtAlwaysEmittedNullWhenNil(t *testing.T) {
	locID := 9
	locKey := "L1"
	it := PublicCurrentLocationItem{
		AssetID:             1,
		AssetExternalKey:    "A1",
		LocationID:          &locID,
		LocationExternalKey: &locKey,
	}
	data, err := json.Marshal(it)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	raw, present := parsed["asset_deleted_at"]
	assert.True(t, present, "asset_deleted_at must always be present (TRA-610)")
	assert.Nil(t, raw, "asset_deleted_at must be JSON null for live assets (TRA-610)")
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
	assert.True(t, got.AssetDeletedAt.Time.Equal(deletedAt))

	data, err := json.Marshal(got)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "2026-04-20T14:00:00.000Z", parsed["asset_deleted_at"])
}
