package asset

import (
	"github.com/trakrf/platform/backend/internal/models/shared"
)

// PublicAssetView is the HTTP shape emitted by read endpoints. It drops
// org_id and carries only the asset's dimension attributes.
//
// TRA-799: the asset's current location is NOT on this shape. Location is
// scan-derived fact data — read it through GET /api/v1/reports/asset-locations
// or GET /api/v1/assets/{asset_id}/history.
//
// description and valid_to are always emitted (null when unset) per
// TRA-610 / BB18 §1.8 audit alignment with PublicLocationView.
//
// deleted_at is always emitted (null for live rows, populated for
// soft-deleted rows surfaced via ?include_deleted=true) per TRA-659 / BB25
// A3. Per-resource views use the unprefixed `deleted_at` (TRA-679 / BB27 S7
// option a) — the prefix was redundant inside the asset namespace. The
// prefixed `asset_deleted_at` is retained only in cross-resource report
// shapes (PublicCurrentLocationItem) where disambiguation matters.
type PublicAssetView struct {
	ID          int                `json:"id"`
	ExternalKey string             `json:"external_key"`
	Name        string             `json:"name"`
	Description *string            `json:"description"`
	Metadata    any                `json:"metadata"`
	IsActive    bool               `json:"is_active"`
	ValidFrom   shared.PublicTime  `json:"valid_from"`
	ValidTo     *shared.PublicTime `json:"valid_to"`
	CreatedAt   shared.PublicTime  `json:"created_at"`
	UpdatedAt   shared.PublicTime  `json:"updated_at"`
	DeletedAt   *shared.PublicTime `json:"deleted_at"`
	Tags        []shared.Tag       `json:"tags"`
}

// ToPublicAssetView projects an AssetView to the public HTTP shape.
func ToPublicAssetView(a AssetView) PublicAssetView {
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
		ID:          a.ID,
		ExternalKey: a.ExternalKey,
		Name:        a.Name,
		Description: desc,
		Metadata:    metadata,
		IsActive:    a.IsActive,
		ValidFrom:   shared.NewPublicTime(a.ValidFrom),
		ValidTo:     shared.PublicTimePtr(a.ValidTo),
		CreatedAt:   shared.NewPublicTime(a.CreatedAt),
		UpdatedAt:   shared.NewPublicTime(a.UpdatedAt),
		DeletedAt:   shared.PublicTimePtr(a.DeletedAt),
		Tags:        a.Tags,
	}
}
