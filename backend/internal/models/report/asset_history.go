package report

import "time"

// AssetInfo contains asset metadata for the response header
type AssetInfo struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

// AssetHistoryItem represents a single scan in the asset's history
type AssetHistoryItem struct {
	Timestamp       time.Time `json:"timestamp"`
	LocationID      *int      `json:"location_id"`
	LocationName    *string   `json:"location_name"`
	DurationSeconds *int      `json:"duration_seconds"`
}

// AssetHistoryResponse is the paginated response for asset history
type AssetHistoryResponse struct {
	Asset      AssetInfo          `json:"asset"`
	Data       []AssetHistoryItem `json:"data"`
	Count      int                `json:"count"`
	Offset     int                `json:"offset"`
	TotalCount int                `json:"total_count"`
}

// AssetHistoryFilter contains query parameters for filtering
type AssetHistoryFilter struct {
	StartDate *time.Time
	EndDate   *time.Time
	Limit     int
	Offset    int
}
