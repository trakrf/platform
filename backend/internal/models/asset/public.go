package asset

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/shared"
)

// PublicAssetView is the HTTP shape emitted by read endpoints. It drops
// org_id and deleted_at and exposes the asset's current location as both
// the canonical int FK and its natural-key external_key (TRA-555).
type PublicAssetView struct {
	ID                         int          `json:"id"`
	ExternalKey                string       `json:"external_key"`
	Name                       string       `json:"name"`
	Description                string       `json:"description,omitempty"`
	CurrentLocationID          *int         `json:"current_location_id"`
	CurrentLocationExternalKey *string      `json:"current_location_external_key"`
	Metadata                   any          `json:"metadata"`
	IsActive                   bool         `json:"is_active"`
	ValidFrom                  time.Time    `json:"valid_from"`
	ValidTo                    *time.Time   `json:"valid_to,omitempty"`
	CreatedAt                  time.Time    `json:"created_at"`
	UpdatedAt                  time.Time    `json:"updated_at"`
	Tags                       []shared.Tag `json:"tags"`
}

// ToPublicAssetView projects an AssetWithLocation to the public HTTP shape.
func ToPublicAssetView(a AssetWithLocation) PublicAssetView {
	// Normalize nil metadata to {} so POST and GET emit the same shape.
	metadata := a.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	return PublicAssetView{
		ID:                         a.ID,
		ExternalKey:                a.ExternalKey,
		Name:                       a.Name,
		Description:                a.Description,
		CurrentLocationID:          a.CurrentLocationID,
		CurrentLocationExternalKey: a.CurrentLocationExternalKey,
		Metadata:                   metadata,
		IsActive:                   a.IsActive,
		ValidFrom:                  a.ValidFrom,
		ValidTo:                    a.ValidTo,
		CreatedAt:                  a.CreatedAt,
		UpdatedAt:                  a.UpdatedAt,
		Tags:                       a.Tags,
	}
}
