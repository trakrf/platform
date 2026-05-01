package report

import "time"

// AssetHistoryItem represents a single scan in the asset's history
type AssetHistoryItem struct {
	Timestamp           time.Time `json:"timestamp"`
	LocationID          *int      `json:"location_id"`
	LocationName        *string   `json:"location_name"`
	LocationExternalKey *string   `json:"location_external_key"`
	DurationSeconds     *int      `json:"duration_seconds"`
}

// AssetHistoryFilter contains query parameters for filtering
type AssetHistoryFilter struct {
	From   *time.Time
	To     *time.Time
	Limit  int
	Offset int
}
