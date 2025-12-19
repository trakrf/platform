package location

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/org"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

type Location struct {
	ID               int        `json:"id"`
	Name             string     `json:"name"`
	OrgID            int        `json:"org_id"`
	Org              *org.Org   `json:"org,omitempty"`
	Identifier       string     `json:"identifier" validate:"required,min=1,max=255"`
	Path             string     `json:"path"`
	Depth            int        `json:"depth"`
	ParentLocationID *int       `json:"parent_location_id"`
	Parent           *Location  `json:"parent,omitempty"`
	Children         []Location `json:"children,omitempty"`
	Ancestors        []Location `json:"ancestors,omitempty"`
	ValidFrom        time.Time  `json:"valid_from"`
	ValidTo          *time.Time `json:"valid_to,omitempty"`
	IsActive         bool       `json:"is_active"`
	Description      string     `json:"description"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        *time.Time `json:"updated_at,omitempty"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
}

type LocationWithRelations struct {
	Location
	Children  []Location `json:"children"`
	Ancestors []Location `json:"ancestors"`
}

type CreateLocationRequest struct {
	Name             string               `json:"name" validate:"required,min=1,max=255" example:"Warehouse 1"`
	Identifier       string               `json:"identifier" validate:"required,min=1,max=255" example:"wh1"`
	ParentLocationID *int                 `json:"parent_location_id,omitempty" validate:"omitempty,min=1" example:"1"`
	Description      string               `json:"description,omitempty" validate:"omitempty,max=1024" example:"Main warehouse location"`
	ValidFrom        shared.FlexibleDate  `json:"valid_from" swaggertype:"string" example:"2025-12-14"`
	ValidTo          *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14"`
	IsActive         bool                 `json:"is_active" example:"true"`
}

type UpdateLocationRequest struct {
	Name             *string              `json:"name,omitempty" validate:"omitempty,min=1,max=255" example:"Warehouse 1"`
	Identifier       *string              `json:"identifier,omitempty" validate:"omitempty,min=1,max=255" example:"wh1"`
	ParentLocationID *int                 `json:"parent_location_id,omitempty" validate:"omitempty,min=1" example:"1"`
	Description      *string              `json:"description,omitempty" validate:"omitempty,max=1024" example:"Updated description"`
	ValidFrom        *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-12-14"`
	ValidTo          *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14"`
	IsActive         *bool                `json:"is_active,omitempty" example:"true"`
}

type LocationListResponse struct {
	Data       []Location        `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}

// LocationView includes tag identifiers for API responses
type LocationView struct {
	Location
	Identifiers []shared.TagIdentifier `json:"identifiers"`
}

// CreateLocationWithIdentifiersRequest extends CreateLocationRequest with optional tag identifiers
type CreateLocationWithIdentifiersRequest struct {
	CreateLocationRequest
	Identifiers []shared.TagIdentifierRequest `json:"identifiers,omitempty" validate:"omitempty,dive"`
}

// LocationViewListResponse is paginated list of LocationViews
type LocationViewListResponse struct {
	Data       []LocationView    `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}
