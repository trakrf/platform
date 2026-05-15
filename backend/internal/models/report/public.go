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
type PublicCurrentLocationItem struct {
	AssetID             int                `json:"asset_id"`
	AssetExternalKey    string             `json:"asset_external_key"`
	LocationID          *int               `json:"location_id"`
	LocationExternalKey *string            `json:"location_external_key"`
	AssetLastSeen       shared.PublicTime  `json:"asset_last_seen"`
	AssetDeletedAt      *shared.PublicTime `json:"asset_deleted_at"`
}

func ToPublicCurrentLocationItem(it CurrentLocationItem) PublicCurrentLocationItem {
	return PublicCurrentLocationItem{
		AssetID:             it.AssetID,
		AssetExternalKey:    it.AssetExternalKey,
		LocationID:          it.LocationID,
		LocationExternalKey: it.LocationExternalKey,
		AssetLastSeen:       shared.NewPublicTime(it.LastSeen),
		AssetDeletedAt:      shared.PublicTimePtr(it.AssetDeletedAt),
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
