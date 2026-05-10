package location

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/shared"
)

// PublicLocationView is the HTTP shape emitted by read endpoints.
//
// description and valid_to are always emitted (null when unset) per
// TRA-610 / BB18 §1.8 — schema asymmetry between sibling PublicXxxView
// types caused integrator-trip failures in stricter generated clients
// (required-vs-omitted-vs-null is three states in languages without
// TS-style optional). The pointer types let nil → JSON null while
// non-nil emits the value.
//
// updated_at is non-nullable (TRA-649 / BB23 S2): the locations table
// declares the column NOT NULL with DEFAULT CURRENT_TIMESTAMP, so every
// row — including freshly minted ones — carries a timestamp. The prior
// `*time.Time` shape generated `Date | null` in typescript-fetch and
// forced null-checks the asset side never required.
type PublicLocationView struct {
	ID                int          `json:"id"`
	ExternalKey       string       `json:"external_key"`
	Name              string       `json:"name"`
	Description       *string      `json:"description"`
	ParentID          *int         `json:"parent_id"`
	ParentExternalKey *string      `json:"parent_external_key"`
	TreePath          string       `json:"tree_path"`
	Depth             int          `json:"depth"`
	IsActive          bool         `json:"is_active"`
	ValidFrom         time.Time    `json:"valid_from"`
	ValidTo           *time.Time   `json:"valid_to"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
	Tags              []shared.Tag `json:"tags"`
}

func ToPublicLocationView(l LocationWithParent) PublicLocationView {
	var desc *string
	if l.Description != "" {
		s := l.Description
		desc = &s
	}
	var updatedAt time.Time
	if l.UpdatedAt != nil {
		updatedAt = *l.UpdatedAt
	}
	return PublicLocationView{
		ID:                l.ID,
		ExternalKey:       l.ExternalKey,
		Name:              l.Name,
		Description:       desc,
		ParentID:          l.ParentID,
		ParentExternalKey: l.ParentExternalKey,
		TreePath:          l.TreePath,
		Depth:             l.Depth,
		IsActive:          l.IsActive,
		ValidFrom:         l.ValidFrom,
		ValidTo:           l.ValidTo,
		CreatedAt:         l.CreatedAt,
		UpdatedAt:         updatedAt,
		Tags:              l.Tags,
	}
}
