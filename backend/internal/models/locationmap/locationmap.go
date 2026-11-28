// Package locationmap holds the location-map shapes: a map image on a parent
// location, and the pins that place its descendant locations on that image.
package locationmap

import "time"

// MaxImageBytes caps a map image (decoded). The migration's CHECK holds the
// same number.
const MaxImageBytes = 5 * 1024 * 1024

// Pin places a location on a map, as a percentage (0–100) of the image's
// width and height rather than pixels, so a re-exported image at another
// resolution keeps every pin in place.
type Pin struct {
	LocationID int     `json:"location_id"`
	XPct       float64 `json:"x_pct"`
	YPct       float64 `json:"y_pct"`
}

// Map is a location's map image metadata plus its pins. The image bytes are
// served separately.
type Map struct {
	LocationID   int       `json:"location_id"`
	LocationName string    `json:"location_name"`
	ContentType  string    `json:"content_type"`
	ByteSize     int       `json:"byte_size"`
	SHA256       string    `json:"sha256"`
	UpdatedAt    time.Time `json:"updated_at"`
	Pins         []Pin     `json:"pins"`
}

// Summary is one entry in the org's list of maps.
type Summary struct {
	LocationID   int       `json:"location_id"`
	LocationName string    `json:"location_name"`
	PinCount     int       `json:"pin_count"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Occupant is a person or asset whose current location is inside a map's
// subtree. LocationID is where it actually is, which may sit below the pin it
// is drawn at.
type Occupant struct {
	AssetID      int       `json:"asset_id"`
	Name         string    `json:"name"`
	ExternalKey  string    `json:"external_key"`
	IsPerson     bool      `json:"is_person"`
	LocationID   int       `json:"location_id"`
	LocationName string    `json:"location_name"`
	LastSeen     time.Time `json:"last_seen"`
}

// ZoneOccupancy is one pinned location and who is in it, including everyone
// in a location below it that has no pin of its own.
type ZoneOccupancy struct {
	LocationID int        `json:"location_id"`
	Name       string     `json:"name"`
	XPct       float64    `json:"x_pct"`
	YPct       float64    `json:"y_pct"`
	Occupants  []Occupant `json:"occupants"`
}

// Presence is the live picture for one map. Unplaced holds occupants inside
// the map's subtree whose location has no pinned ancestor-or-self, so nobody
// on site silently drops off the map.
type Presence struct {
	MapLocationID int             `json:"map_location_id"`
	GeneratedAt   time.Time       `json:"generated_at"`
	Zones         []ZoneOccupancy `json:"zones"`
	Unplaced      []Occupant      `json:"unplaced"`
}
