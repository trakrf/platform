package asset

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/shared"
)

// PublicAssetView is the HTTP shape emitted by read endpoints. It drops
// org_id and deleted_at and exposes the asset's location as both the
// canonical int FK and its natural-key external_key (TRA-555). The wire
// fields are `location_id` and `location_external_key` (TRA-580 C-3 dropped
// the `current_` prefix that conflicted with the report row shape).
//
// description and valid_to are always emitted (null when unset) per
// TRA-610 / BB18 §1.8 audit alignment with PublicLocationView.
type PublicAssetView struct {
	ID                  int          `json:"id"`
	ExternalKey         string       `json:"external_key"`
	Name                string       `json:"name"`
	Description         *string      `json:"description"`
	LocationID          *int         `json:"location_id"`
	LocationExternalKey *string      `json:"location_external_key"`
	Metadata            any          `json:"metadata"`
	IsActive            bool         `json:"is_active"`
	ValidFrom           time.Time    `json:"valid_from"`
	ValidTo             *time.Time   `json:"valid_to"`
	CreatedAt           time.Time    `json:"created_at"`
	UpdatedAt           time.Time    `json:"updated_at"`
	Tags                []shared.Tag `json:"tags"`
}

// ToPublicAssetView projects an AssetWithLocation to the public HTTP shape.
func ToPublicAssetView(a AssetWithLocation) PublicAssetView {
	// Normalize nil metadata to {} so POST and GET emit the same shape.
	metadata := a.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	var desc *string
	if a.Description != "" {
		s := a.Description
		desc = &s
	}
	return PublicAssetView{
		ID:                  a.ID,
		ExternalKey:         a.ExternalKey,
		Name:                a.Name,
		Description:         desc,
		LocationID:          a.LocationID,
		LocationExternalKey: a.LocationExternalKey,
		Metadata:            metadata,
		IsActive:            a.IsActive,
		ValidFrom:           a.ValidFrom,
		ValidTo:             a.ValidTo,
		CreatedAt:           a.CreatedAt,
		UpdatedAt:           a.UpdatedAt,
		Tags:                a.Tags,
	}
}
