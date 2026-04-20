package asset

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/shared"
)

// PublicAssetView is the HTTP shape emitted by read endpoints. It drops
// org_id and deleted_at, renames the surrogate id, and carries the parent
// location's natural key instead of the INT FK.
type PublicAssetView struct {
	Identifier      string                 `json:"identifier"`
	Name            string                 `json:"name"`
	Type            string                 `json:"type,omitempty"`
	Description     string                 `json:"description,omitempty"`
	CurrentLocation *string                `json:"current_location,omitempty"`
	Metadata        any                    `json:"metadata,omitempty"`
	IsActive        bool                   `json:"is_active"`
	ValidFrom       time.Time              `json:"valid_from"`
	ValidTo         *time.Time             `json:"valid_to,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	SurrogateID     int                    `json:"surrogate_id"`
	Identifiers     []shared.TagIdentifier `json:"identifiers"`
}

// ToPublicAssetView projects an AssetWithLocation to the public HTTP shape.
func ToPublicAssetView(a AssetWithLocation) PublicAssetView {
	return PublicAssetView{
		Identifier:      a.Identifier,
		Name:            a.Name,
		Type:            a.Type,
		Description:     a.Description,
		CurrentLocation: a.CurrentLocationIdentifier,
		Metadata:        a.Metadata,
		IsActive:        a.IsActive,
		ValidFrom:       a.ValidFrom,
		ValidTo:         a.ValidTo,
		CreatedAt:       a.CreatedAt,
		UpdatedAt:       a.UpdatedAt,
		SurrogateID:     a.ID,
		Identifiers:     a.Identifiers,
	}
}
