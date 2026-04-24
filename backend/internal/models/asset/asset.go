package asset

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/org"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

type Asset struct {
	ID                int        `json:"id"`
	OrgID             int        `json:"org_id"`
	Org               *org.Org   `json:"org"`
	Identifier        string     `json:"identifier"`
	Name              string     `json:"name"`
	Type              string     `json:"type" example:"asset" enums:"asset,person,inventory" extensions:"x-extensible-enum=true"`
	Description       string     `json:"description"`
	CurrentLocationID *int       `json:"current_location_id"`
	ValidFrom         time.Time  `json:"valid_from"`
	ValidTo           *time.Time `json:"valid_to"`
	Metadata          any        `json:"metadata"`
	IsActive          bool       `json:"is_active"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `json:"deleted_at"`
}

type CreateAssetRequest struct {
	OrgID             int                  `json:"-" swaggerignore:"true"`
	Identifier        string               `json:"identifier,omitempty" validate:"omitempty,max=255"`
	Name              string               `json:"name" validate:"required,min=1,max=255"`
	Type              string               `json:"type,omitempty" validate:"omitempty,oneof=asset person inventory" enums:"asset,person,inventory" example:"asset"`
	Description       string               `json:"description,omitempty" validate:"omitempty,max=1024"`
	CurrentLocationID *int                 `json:"current_location_id,omitempty" swaggerignore:"true" validate:"omitempty,min=1"`
	CurrentLocation   *string              `json:"current_location,omitempty" validate:"omitempty,min=1,max=255"`
	ValidFrom         *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-01-01"`
	ValidTo           *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-01-01"`
	Metadata          any                  `json:"metadata,omitempty"`
	IsActive          *bool                `json:"is_active,omitempty" example:"true"`
}

type UpdateAssetRequest struct {
	Identifier        *string              `json:"identifier" validate:"omitempty,min=1,max=255"`
	Name              *string              `json:"name" validate:"omitempty,min=1,max=255"`
	Type              *string              `json:"type,omitempty" validate:"omitempty,oneof=asset person inventory" enums:"asset,person,inventory"`
	Description       *string              `json:"description" validate:"omitempty,max=1024"`
	CurrentLocationID *int                 `json:"current_location_id" swaggerignore:"true"`
	CurrentLocation   *string              `json:"current_location,omitempty" validate:"omitempty,min=1,max=255"`
	ValidFrom         *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-01-01"`
	ValidTo           *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-01-01"`
	// Set by the PUT handler when the body had `"valid_to": null`, to request
	// an SQL NULL write. Not decoded from JSON directly.
	ClearValidTo bool  `json:"-" swaggerignore:"true"`
	Metadata     *any  `json:"metadata"`
	IsActive     *bool `json:"is_active"`
}

type AssetListResponse struct {
	Data       []Asset           `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}

type AssetView struct {
	Asset
	Identifiers []shared.TagIdentifier `json:"identifiers"`
}

type CreateAssetWithIdentifiersRequest struct {
	CreateAssetRequest
	Identifiers []shared.TagIdentifierRequest `json:"identifiers,omitempty" validate:"omitempty,dive"`
}

type AssetViewListResponse struct {
	Data       []AssetView       `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}

// AssetWithLocation is AssetView plus the resolved parent-location natural key.
// Populated by GetAssetByIdentifier / list-with-join storage methods; returned
// to HTTP handlers which then project it to PublicAssetView.
type AssetWithLocation struct {
	AssetView
	CurrentLocationIdentifier *string `json:"current_location_identifier,omitempty"`
}

// ListFilter carries the optional filters the assets list endpoint supports.
type ListFilter struct {
	LocationIdentifiers []string // OR semantics when multi-valued
	IsActive            *bool
	Type                *string
	Q                   *string // substring match (case-insensitive) on name, identifier, description, and active identifier values
	Sorts               []ListSort
	Limit               int
	Offset              int
}

// ListSort is one (field, direction) entry.
type ListSort struct {
	Field string
	Desc  bool
}
