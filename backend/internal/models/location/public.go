package location

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/shared"
)

type PublicLocationView struct {
	ID                int          `json:"id"`
	ExternalKey       string       `json:"external_key"`
	Name              string       `json:"name"`
	Description       string       `json:"description,omitempty"`
	ParentID          *int         `json:"parent_id"`
	ParentExternalKey *string      `json:"parent_external_key"`
	TreePath          string       `json:"tree_path"`
	Depth             int          `json:"depth"`
	IsActive          bool         `json:"is_active"`
	ValidFrom         time.Time    `json:"valid_from"`
	ValidTo           *time.Time   `json:"valid_to,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         *time.Time   `json:"updated_at,omitempty"`
	Tags              []shared.Tag `json:"tags"`
}

func ToPublicLocationView(l LocationWithParent) PublicLocationView {
	return PublicLocationView{
		ID:                l.ID,
		ExternalKey:       l.ExternalKey,
		Name:              l.Name,
		Description:       l.Description,
		ParentID:          l.ParentID,
		ParentExternalKey: l.ParentExternalKey,
		TreePath:          l.TreePath,
		Depth:             l.Depth,
		IsActive:          l.IsActive,
		ValidFrom:         l.ValidFrom,
		ValidTo:           l.ValidTo,
		CreatedAt:         l.CreatedAt,
		UpdatedAt:         l.UpdatedAt,
		Tags:              l.Tags,
	}
}
