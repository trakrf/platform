package report

import "time"

// PublicCurrentLocationItem is the public shape for /api/v1/locations/current items.
type PublicCurrentLocationItem struct {
	Asset    string    `json:"asset"`
	Location string    `json:"location"`
	LastSeen time.Time `json:"last_seen"`
}

func ToPublicCurrentLocationItem(it CurrentLocationItem) PublicCurrentLocationItem {
	return PublicCurrentLocationItem{
		Asset:    it.AssetIdentifier,
		Location: derefString(it.LocationIdentifier),
		LastSeen: it.LastSeen,
	}
}

// PublicAssetHistoryItem is the public shape for asset-history list items.
type PublicAssetHistoryItem struct {
	Timestamp       time.Time `json:"timestamp"`
	Location        string    `json:"location"`
	DurationSeconds *int      `json:"duration_seconds,omitempty"`
}

func ToPublicAssetHistoryItem(it AssetHistoryItem) PublicAssetHistoryItem {
	return PublicAssetHistoryItem{
		Timestamp:       it.Timestamp,
		Location:        derefString(it.LocationIdentifier),
		DurationSeconds: it.DurationSeconds,
	}
}

// derefString safely dereferences a *string, returning empty string if nil.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
