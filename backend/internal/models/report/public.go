package report

import "time"

// PublicCurrentLocationItem is the public shape for /api/v1/locations/current items.
type PublicCurrentLocationItem struct {
	Asset              string     `json:"asset"`
	LocationIdentifier string     `json:"location_identifier"`
	LastSeen           time.Time  `json:"last_seen"`
	AssetDeletedAt     *time.Time `json:"asset_deleted_at,omitempty"`
}

func ToPublicCurrentLocationItem(it CurrentLocationItem) PublicCurrentLocationItem {
	return PublicCurrentLocationItem{
		Asset:              it.AssetIdentifier,
		LocationIdentifier: derefString(it.LocationIdentifier),
		LastSeen:           it.LastSeen,
		AssetDeletedAt:     it.AssetDeletedAt,
	}
}

// PublicAssetHistoryItem is the public shape for asset-history list items.
type PublicAssetHistoryItem struct {
	Timestamp          time.Time `json:"timestamp"`
	LocationIdentifier string    `json:"location_identifier"`
	DurationSeconds    *int      `json:"duration_seconds"`
}

func ToPublicAssetHistoryItem(it AssetHistoryItem) PublicAssetHistoryItem {
	return PublicAssetHistoryItem{
		Timestamp:          it.Timestamp,
		LocationIdentifier: derefString(it.LocationIdentifier),
		DurationSeconds:    it.DurationSeconds,
	}
}

// derefString safely dereferences a *string, returning empty string if nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
