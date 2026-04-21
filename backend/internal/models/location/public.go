package location

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/shared"
)

type PublicLocationView struct {
	Identifier  string                 `json:"identifier"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parent      *string                `json:"parent,omitempty"`
	Path        string                 `json:"path"`
	Depth       int                    `json:"depth"`
	IsActive    bool                   `json:"is_active"`
	ValidFrom   time.Time              `json:"valid_from"`
	ValidTo     *time.Time             `json:"valid_to,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   *time.Time             `json:"updated_at,omitempty"`
	SurrogateID int                    `json:"surrogate_id"`
	Identifiers []shared.TagIdentifier `json:"identifiers"`
}

func ToPublicLocationView(l LocationWithParent) PublicLocationView {
	return PublicLocationView{
		Identifier:  l.Identifier,
		Name:        l.Name,
		Description: l.Description,
		Parent:      l.ParentIdentifier,
		Path:        l.Path,
		Depth:       l.Depth,
		IsActive:    l.IsActive,
		ValidFrom:   l.ValidFrom,
		ValidTo:     l.ValidTo,
		CreatedAt:   l.CreatedAt,
		UpdatedAt:   l.UpdatedAt,
		SurrogateID: l.ID,
		Identifiers: l.Identifiers,
	}
}

// ToPublicLocationViewFromLocation adapts a bare Location (as returned by hierarchy
// queries that don't carry the parent identifier or per-location tag identifiers) into
// the same public shape used by GET /locations/{identifier}. Parent stays nil and
// Identifiers is an empty slice so the JSON renders `[]` rather than `null`.
func ToPublicLocationViewFromLocation(l Location) PublicLocationView {
	return PublicLocationView{
		Identifier:  l.Identifier,
		Name:        l.Name,
		Description: l.Description,
		Path:        l.Path,
		Depth:       l.Depth,
		IsActive:    l.IsActive,
		ValidFrom:   l.ValidFrom,
		ValidTo:     l.ValidTo,
		CreatedAt:   l.CreatedAt,
		UpdatedAt:   l.UpdatedAt,
		SurrogateID: l.ID,
		Identifiers: []shared.TagIdentifier{},
	}
}
