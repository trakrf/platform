package location

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/org"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

type Location struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	OrgID       int        `json:"org_id"`
	Org         *org.Org   `json:"org,omitempty"`
	ExternalKey string     `json:"external_key" validate:"required,min=1,max=255"`
	TreePath    string     `json:"tree_path"`
	Depth       int        `json:"depth"`
	ParentID    *int       `json:"parent_id"`
	Parent      *Location  `json:"parent,omitempty"`
	Children    []Location `json:"children,omitempty"`
	Ancestors   []Location `json:"ancestors,omitempty"`
	ValidFrom   time.Time  `json:"valid_from"`
	ValidTo     *time.Time `json:"valid_to,omitempty"`
	IsActive    bool       `json:"is_active"`
	Description string     `json:"description"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
}

type LocationWithRelations struct {
	Location
	Children  []Location `json:"children"`
	Ancestors []Location `json:"ancestors"`
}

type CreateLocationRequest struct {
	Name              string               `json:"name" validate:"required,min=1,max=255" example:"Warehouse 1"`
	ExternalKey       string               `json:"external_key" validate:"required,min=1,max=255" example:"wh1"`
	ParentID          *int                 `json:"parent_id,omitempty" validate:"omitempty,min=1" example:"42"`
	ParentExternalKey *string              `json:"parent_external_key,omitempty" validate:"omitempty,min=1,max=255" example:"wh1"`
	Description       string               `json:"description,omitempty" validate:"omitempty,max=1024" example:"Main warehouse location"`
	ValidFrom         *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-12-14T00:00:00Z"`
	ValidTo           *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14T00:00:00Z"`
	IsActive          *bool                `json:"is_active,omitempty" example:"true"`
}

// UpdateLocationRequest is the PUT body. Strict-decoded — unknown fields
// (including the read-only fields on PublicLocationView like id, created_at,
// updated_at, tree_path, depth, tags) produce a 400. See the same comment
// on asset.UpdateAssetRequest for the TRA-587 / TRA-592 context.
type UpdateLocationRequest struct {
	Name              *string              `json:"name,omitempty" validate:"omitempty,min=1,max=255" example:"Warehouse 1"`
	ExternalKey       *string              `json:"external_key,omitempty" validate:"omitempty,min=1,max=255" example:"wh1"`
	ParentID          *int                 `json:"parent_id,omitempty" validate:"omitempty,min=1" example:"42"`
	ParentExternalKey *string              `json:"parent_external_key,omitempty" validate:"omitempty,min=1,max=255" example:"wh1"`
	Description       *string              `json:"description,omitempty" validate:"omitempty,max=1024" example:"Updated description"`
	ValidFrom         *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-12-14T00:00:00Z"`
	ValidTo           *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14T00:00:00Z"`
	// Set by the PUT handler when the body had `"valid_to": null`, to request
	// an SQL NULL write. Not decoded from JSON directly.
	ClearValidTo bool  `json:"-" swaggerignore:"true"`
	IsActive     *bool `json:"is_active,omitempty" example:"true"`
}

type LocationListResponse struct {
	Data       []Location        `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}

// LocationView includes tags for API responses
type LocationView struct {
	Location
	Tags []shared.Tag `json:"tags"`
}

// CreateLocationWithTagsRequest extends CreateLocationRequest with optional tags
type CreateLocationWithTagsRequest struct {
	CreateLocationRequest
	Tags []shared.TagRequest `json:"tags,omitempty" validate:"omitempty,dive"`
}

// LocationViewListResponse is paginated list of LocationViews
type LocationViewListResponse struct {
	Data       []LocationView    `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}

// LocationWithParent is LocationView plus the resolved parent's natural key.
type LocationWithParent struct {
	LocationView
	ParentExternalKey *string `json:"parent_external_key,omitempty"`
}

// ListFilter carries the optional filters the locations list endpoint supports.
//
// ParentIDs and ParentExternalKeys are mutually exclusive at the handler
// boundary; the storage layer ANDs both into the WHERE clause if a caller
// somehow passed both, but the handler rejects that combination.
type ListFilter struct {
	ParentIDs          []int
	ParentExternalKeys []string
	// Equality match on l.external_key (any-of). Single value yields the
	// natural-key lookup that lives on the collection per TRA-600.
	ExternalKeys []string
	IsActive     *bool
	Q            *string
	Sorts        []ListSort
	Limit        int
	Offset       int
}

type ListSort struct {
	Field string
	Desc  bool
}
