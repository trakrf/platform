package report

import "time"

// AssetHistoryItem is one stay in the asset's history: an unbroken run of
// observations at one location, not a single scan. Timestamp is when the stay
// began; DurationSeconds runs to the next observation elsewhere, nil while the
// stay is ongoing.
type AssetHistoryItem struct {
	Timestamp           time.Time `json:"timestamp"`
	LocationID          *int      `json:"location_id"`
	LocationName        *string   `json:"location_name"`
	LocationExternalKey *string   `json:"location_external_key"`
	DurationSeconds     *int      `json:"duration_seconds"`
}

// AssetHistorySort is a single (field, direction) clause as parsed from
// the ?sort= query parameter.
type AssetHistorySort struct {
	Field string
	Desc  bool
}

// AssetHistoryFilter contains query parameters for filtering
type AssetHistoryFilter struct {
	From   *time.Time
	To     *time.Time
	Limit  int
	Offset int
	Sorts  []AssetHistorySort
}
