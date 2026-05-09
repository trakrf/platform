package report

import "time"

// CurrentLocationItem represents a single asset's current location (internal projection)
type CurrentLocationItem struct {
	AssetID             int        `json:"asset_id"`
	AssetName           string     `json:"asset_name"`
	AssetExternalKey    string     `json:"asset_external_key"`
	LocationID          *int       `json:"location_id"`
	LocationName        *string    `json:"location_name"`
	LocationExternalKey *string    `json:"location_external_key"`
	LastSeen            time.Time  `json:"last_seen"`
	AssetDeletedAt      *time.Time `json:"asset_deleted_at,omitempty"`
}

// CurrentLocationSort declares one entry in a list-endpoint sort. Field is
// one of the documented enum values for /locations/current; Desc is true for
// "-prefixed" entries.
type CurrentLocationSort struct {
	Field string
	Desc  bool
}

// CurrentLocationFilter contains query parameters for filtering
type CurrentLocationFilter struct {
	LocationIDs          []int    // filter by canonical location id(s)
	LocationExternalKeys []string // filter by location external_key(s)
	Q                    *string  // substring search (case-insensitive) on asset name, external_key, and active tag values
	IncludeDeleted       bool     // when true, includes rows for soft-deleted assets (default false)
	Sorts                []CurrentLocationSort
	Limit                int
	Offset               int
}
