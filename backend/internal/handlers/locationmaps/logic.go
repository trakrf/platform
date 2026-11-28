package locationmaps

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/trakrf/platform/backend/internal/models/locationmap"
)

var (
	errEmptyImage       = errors.New("image is empty")
	errImageTooLarge    = fmt.Errorf("image must be at most %d bytes", locationmap.MaxImageBytes)
	errUnsupportedImage = errors.New("image must be a PNG, JPEG or WebP")
)

// allowedImageTypes are the types a map image may be. SVG is deliberately
// absent: it can carry script, and nothing on a floor plan needs vectors.
var allowedImageTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
}

// sniffMapImage returns the image's content type from its bytes. Whatever
// type the client declared is ignored.
func sniffMapImage(b []byte) (string, error) {
	if len(b) == 0 {
		return "", errEmptyImage
	}
	if len(b) > locationmap.MaxImageBytes {
		return "", errImageTooLarge
	}
	ct := http.DetectContentType(b)
	if !allowedImageTypes[ct] {
		return "", errUnsupportedImage
	}
	return ct, nil
}

// validatePins checks shape only: coordinate ranges, duplicates, and the map
// location pinned on its own map. That each pin is a live descendant of the
// map location needs storage and is checked by the handler.
func validatePins(mapLocationID int, pins []locationmap.Pin) error {
	seen := make(map[int]bool, len(pins))
	for _, p := range pins {
		if p.XPct < 0 || p.XPct > 100 || p.YPct < 0 || p.YPct > 100 {
			return errors.New("pin x_pct and y_pct must be between 0 and 100")
		}
		if p.LocationID == mapLocationID {
			return errors.New("a location cannot be pinned on its own map")
		}
		if seen[p.LocationID] {
			return fmt.Errorf("location %d is pinned more than once", p.LocationID)
		}
		seen[p.LocationID] = true
	}
	return nil
}

// rollUp finds the pin a location is drawn at: the location itself when it is
// pinned, else its nearest pinned ancestor below the map location. ok is false
// when the walk reaches the map location (or leaves the subtree) without
// meeting a pin. parents maps every location in the subtree to its parent.
func rollUp(mapLocationID int, parents map[int]*int, pinned map[int]bool, locationID int) (int, bool) {
	cur := locationID
	// The step cap guards against a parent cycle the location writes forbid
	// but this walk cannot assume away.
	for steps := 0; steps <= len(parents); steps++ {
		if cur == mapLocationID {
			return 0, false
		}
		if pinned[cur] {
			return cur, true
		}
		parent, inSubtree := parents[cur]
		if !inSubtree || parent == nil {
			return 0, false
		}
		cur = *parent
	}
	return 0, false
}
