package report

import "time"

// CurrentLocationItem represents a single asset's current location
type CurrentLocationItem struct {
	AssetID            int       `json:"asset_id"`
	AssetName          string    `json:"asset_name"`
	AssetIdentifier    string    `json:"asset_identifier"`
	LocationID         *int      `json:"location_id"`         // nullable
	LocationName       *string   `json:"location_name"`       // nullable
	LocationIdentifier *string   `json:"location_identifier"` // nullable
	LastSeen           time.Time `json:"last_seen"`
}

// CurrentLocationsResponse is the paginated response for current locations
type CurrentLocationsResponse struct {
	Data       []CurrentLocationItem `json:"data"`
	Count      int                   `json:"count"`
	Offset     int                   `json:"offset"`
	TotalCount int                   `json:"total_count"`
}

// CurrentLocationFilter contains query parameters for filtering
type CurrentLocationFilter struct {
	LocationIdentifiers []string // filter by location natural key(s)
	Q                   *string  // substring match (case-insensitive) on asset name, identifier, and active identifier values
	Limit               int
	Offset              int
}
