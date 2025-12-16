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
	Type              string     `json:"type" `
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
	OrgID             int        `json:"org_id" validate:"omitempty,min=1"`
	Identifier        string     `json:"identifier" validate:"required,min=1,max=255"`
	Name              string     `json:"name" validate:"required,min=1,max=255"`
	Type              string     `json:"type" validate:"oneof=asset"`
	Description       string     `json:"description" validate:"omitempty,max=1024"`
	CurrentLocationID *int       `json:"current_location_id" validate:"omitempty,min=1"`
	ValidFrom         time.Time  `json:"valid_from"`
	ValidTo           *time.Time `json:"valid_to"`
	Metadata          any        `json:"metadata"`
	IsActive          bool       `json:"is_active"`
}

type UpdateAssetRequest struct {
	OrgID             *int       `json:"org_id" validate:"omitempty,min=1"`
	Identifier        *string    `json:"identifier" validate:"omitempty,min=1,max=255"`
	Name              *string    `json:"name" validate:"omitempty,min=1,max=255"`
	Type              *string    `json:"type" validate:"omitempty,oneof=asset"`
	Description       *string    `json:"description" validate:"omitempty,max=1024"`
	CurrentLocationID *int       `json:"current_location_id"`
	ValidFrom         *time.Time `json:"valid_from"`
	ValidTo           *time.Time `json:"valid_to"`
	Metadata          *any       `json:"metadata"`
	IsActive          *bool      `json:"is_active"`
}

type AssetListResponse struct {
	Data       []Asset           `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}

// AssetView is the API response model that includes embedded tag identifiers.
// GET endpoints return this instead of raw Asset.
type AssetView struct {
	Asset
	Identifiers []shared.TagIdentifier `json:"identifiers"`
}

// CreateAssetWithIdentifiersRequest extends CreateAssetRequest with optional identifiers.
type CreateAssetWithIdentifiersRequest struct {
	CreateAssetRequest
	Identifiers []shared.TagIdentifierRequest `json:"identifiers,omitempty" validate:"omitempty,dive"`
}

// AssetViewListResponse wraps a list of AssetViews with pagination.
type AssetViewListResponse struct {
	Data       []AssetView       `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}
