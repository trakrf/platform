package webhook

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/trakrf/platform/backend/internal/assetevent"
)

// Envelope is the wire projection of a domain event.
//
// Logical data only: the obfuscated id is wire-canonical and external_key is
// the natural-key alternate. No scan_point_id, no EPC, no tag_scan_id — the
// physical layer stays internal.
//
// There is no sequence field. Concurrent MQTT messages mean deliveries can
// arrive out of order, and a counter would imply an ordering guarantee we do
// not provide; occurred_at is what consumers order by.
type Envelope struct {
	Event      string    `json:"event"`
	DeliveryID string    `json:"delivery_id"`
	OccurredAt time.Time `json:"occurred_at"`
	Data       Data      `json:"data"`
}

// Data is the asset.moved body.
type Data struct {
	Asset AssetRef `json:"asset"`
	// FromLocation is null for a genuine first-ever sighting.
	FromLocation *LocationRef `json:"from_location"`
	ToLocation   LocationRef  `json:"to_location"`
}

// AssetRef is an asset's logical identity on the wire.
type AssetRef struct {
	ID          int    `json:"id"`
	ExternalKey string `json:"external_key"`
	Name        string `json:"name"`
}

// LocationRef is a location's logical identity on the wire.
type LocationRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// NewEnvelope projects a domain event onto the wire shape.
func NewEnvelope(ev assetevent.AssetMoved) Envelope {
	env := Envelope{
		Event:      assetevent.EventAssetMoved,
		DeliveryID: ev.DeliveryID,
		OccurredAt: ev.OccurredAt.UTC(),
		Data: Data{
			Asset: AssetRef{
				ID:          ev.Asset.ID,
				ExternalKey: ev.Asset.ExternalKey,
				Name:        ev.Asset.Name,
			},
			ToLocation: LocationRef{ID: ev.To.ID, Name: ev.To.Name},
		},
	}
	if ev.From != nil {
		env.Data.FromLocation = &LocationRef{ID: ev.From.ID, Name: ev.From.Name}
	}
	return env
}

// Encode renders the delivery body.
func Encode(ev assetevent.AssetMoved) ([]byte, error) {
	return json.Marshal(NewEnvelope(ev))
}

// SyntheticEvent builds the placeholder asset.moved that POST
// /api/v1/webhooks/{id}/test fires, so an integrator can prove their endpoint
// and signature verification work before any real asset moves.
//
// The ids are obviously-fake small integers and the names say so: a test fire
// must never be mistaken for a real movement in a customer's system.
func SyntheticEvent(orgID int) assetevent.AssetMoved {
	from := assetevent.Location{ID: 1, Name: "Test Origin"}
	return assetevent.AssetMoved{
		DeliveryID: uuid.NewString(),
		OccurredAt: time.Now().UTC(),
		OrgID:      orgID,
		Asset: assetevent.Asset{
			ID:          1,
			ExternalKey: "TEST-ASSET",
			Name:        "Test Asset (webhook test fire)",
		},
		From: &from,
		To:   assetevent.Location{ID: 2, Name: "Test Destination"},
	}
}
