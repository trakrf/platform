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
