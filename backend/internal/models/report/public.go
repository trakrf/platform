package report

import (
	"github.com/trakrf/platform/backend/internal/models/shared"
)

// PublicCurrentLocationItem is the public shape for /api/v1/reports/asset-locations items.
//
// asset_deleted_at is always emitted (null when the asset is live) per the
// TRA-610 / BB18 §1.8 + §1.10 audit alignment. The default ?include_deleted
// is false, so most rows return null; passing include_deleted=true populates
// it for soft-deleted assets.
//
// TRA-732 R4 / BB39 F8: asset_id and asset_external_key are non-nullable.
// Every row in this view originates from a live (or deletion-included) row in
// trakrf.assets, which has both columns NOT NULL. Earlier pointer types here
// were vestigial — they were always dereferenced from the source values, never
// nil. Tightening the spec lets generated SDKs surface the fields as
// non-optional and lets integrators drop dead null-checks.
//
// TRA-1023: dwell_started_at / dwell_seconds describe the asset's current
// presence interval — how long it has been at location_id without moving.
// Both are non-nullable for the same reason asset_last_seen is: every row here
// is derived from at least one materialized scan bucket.
//
// dwell_seconds is an OBSERVED span (asset_last_seen - dwell_started_at), not a
// wall clock. It freezes when reads stop rather than growing forever for an
// asset that has left. Consumers that need "is this still true?" should read
// asset_last_seen on the same row and apply their own staleness rule; this
// report deliberately does not bake in the geofence engine's age_out_seconds,
// which is a per-output-device knob with no meaning in an asset-scoped view.
type PublicCurrentLocationItem struct {
	AssetID             int                `json:"asset_id"`
	AssetExternalKey    string             `json:"asset_external_key"`
	LocationID          *int               `json:"location_id"`
	LocationExternalKey *string            `json:"location_external_key"`
	AssetLastSeen       shared.PublicTime  `json:"asset_last_seen"`
	AssetDeletedAt      *shared.PublicTime `json:"asset_deleted_at"`
	DwellStartedAt      shared.PublicTime  `json:"dwell_started_at"`
	// int64 mirrors the BIGINT the query casts to; the spec advertises it as
	// int32, matching PublicAssetHistoryItem.duration_seconds. That is the
	// deliberate convention, not an oversight — apispec's int64 roster
	// (surrogateIDFields) is for surrogate ids only, and non-id integers are
	// explicitly excluded from it per TRA-864.
	DwellSeconds int64 `json:"dwell_seconds"`
}

func ToPublicCurrentLocationItem(it CurrentLocationItem) PublicCurrentLocationItem {
	return PublicCurrentLocationItem{
		AssetID:             it.AssetID,
		AssetExternalKey:    it.AssetExternalKey,
		LocationID:          it.LocationID,
		LocationExternalKey: it.LocationExternalKey,
		AssetLastSeen:       shared.NewPublicTime(it.LastSeen),
		AssetDeletedAt:      shared.PublicTimePtr(it.AssetDeletedAt),
		DwellStartedAt:      shared.NewPublicTime(it.DwellStartedAt),
		DwellSeconds:        it.DwellSeconds,
	}
}

// PublicAssetHistoryItem is the public shape for asset-history list items.
type PublicAssetHistoryItem struct {
	EventObservedAt     shared.PublicTime `json:"event_observed_at"`
	LocationID          *int              `json:"location_id"`
	LocationExternalKey *string           `json:"location_external_key"`
	DurationSeconds     *int              `json:"duration_seconds"`
}

func ToPublicAssetHistoryItem(it AssetHistoryItem) PublicAssetHistoryItem {
	return PublicAssetHistoryItem{
		EventObservedAt:     shared.NewPublicTime(it.Timestamp),
		LocationID:          it.LocationID,
		LocationExternalKey: it.LocationExternalKey,
		DurationSeconds:     it.DurationSeconds,
	}
}
