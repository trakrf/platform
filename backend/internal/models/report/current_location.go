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

	// DwellStartedAt is the first observation of this asset at its current
	// location within the current unbroken run, and DwellSeconds is
	// LastSeen - DwellStartedAt (TRA-1023). Both are non-pointer: every row in
	// this projection originates from at least one asset_scan_latest bucket, so
	// the run always has a first bucket and the span is always defined.
	//
	// DwellSeconds is int64 to mirror the BIGINT the query casts to. The public
	// spec advertises dwell_seconds as int32, matching
	// PublicAssetHistoryItem.duration_seconds — that is the deliberate
	// convention, not an oversight: apispec's int64 roster (surrogateIDFields)
	// covers surrogate ids only and explicitly excludes non-id integers per
	// TRA-864. Keep this note here rather than on the public struct; swaggo
	// lifts field doc comments straight into the published schema description,
	// and this is an internal detail.
	DwellStartedAt time.Time `json:"dwell_started_at"`
	DwellSeconds   int64     `json:"dwell_seconds"`
}

// CurrentLocationSort declares one entry in a list-endpoint sort. Field is
// one of the documented enum values for /reports/asset-locations; Desc is true for
// "-prefixed" entries.
type CurrentLocationSort struct {
	Field string
	Desc  bool
}

// CurrentLocationFilter contains query parameters for filtering
type CurrentLocationFilter struct {
	LocationIDs          []int    // filter by canonical location id(s)
	LocationExternalKeys []string // filter by location external_key(s)
	AssetIDs             []int    // filter by canonical asset id(s)
	AssetExternalKeys    []string // filter by asset external_key(s)
	Q                    *string  // substring search (case-insensitive) on asset name, external_key, and active tag values
	IncludeDeleted       bool     // when true, includes rows for soft-deleted assets (default false)
	Sorts                []CurrentLocationSort
	Limit                int
	Offset               int
}
