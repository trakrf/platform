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
type PublicCurrentLocationItem struct {
	AssetID             *int               `json:"asset_id"`
	AssetExternalKey    *string            `json:"asset_external_key"`
	LocationID          *int               `json:"location_id"`
	LocationExternalKey *string            `json:"location_external_key"`
	AssetLastSeen       shared.PublicTime  `json:"asset_last_seen"`
	AssetDeletedAt      *shared.PublicTime `json:"asset_deleted_at"`
}

func ToPublicCurrentLocationItem(it CurrentLocationItem) PublicCurrentLocationItem {
	assetID := it.AssetID
	assetKey := it.AssetExternalKey
	return PublicCurrentLocationItem{
		AssetID:             &assetID,
		AssetExternalKey:    &assetKey,
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
