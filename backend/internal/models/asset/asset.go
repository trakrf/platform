package asset

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/org"
	"github.com/trakrf/platform/backend/internal/models/shared"
)

type Asset struct {
	ID          int        `json:"id"`
	OrgID       int        `json:"org_id"`
	Org         *org.Org   `json:"org"`
	ExternalKey string     `json:"external_key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	LocationID  *int       `json:"location_id"`
	ValidFrom   time.Time  `json:"valid_from"`
	ValidTo     *time.Time `json:"valid_to"`
	Metadata    any        `json:"metadata"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at"`
}

type CreateAssetRequest struct {
	OrgID               int                  `json:"-" swaggerignore:"true"`
	ExternalKey         string               `json:"external_key,omitempty" validate:"omitempty,max=255"`
	Name                string               `json:"name" validate:"required,min=1,max=255"`
	Description         string               `json:"description,omitempty" validate:"omitempty,max=1024"`
	LocationID          *int                 `json:"location_id,omitempty" example:"42"`
	LocationExternalKey *string              `json:"location_external_key,omitempty" validate:"omitempty,min=1,max=255" example:"WHS-01"`
	ValidFrom           *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-01-01T00:00:00Z"`
	ValidTo             *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-01-01T00:00:00Z"`
	Metadata            any                  `json:"metadata,omitempty"`
	IsActive            *bool                `json:"is_active,omitempty" example:"true"`
}

// UpdateAssetRequest is the PUT body. The handler decodes it via
// DecodeJSONStrict, so unknown fields (including the read-only fields on
// PublicAssetView like id, created_at, updated_at, tags) produce a 400.
// TRA-587 / BB16 S8 considered relaxing this to silently ignore read-only
// fields (Stripe/GitHub style) for sync-job ergonomics; we kept strict
// reject because the round-trip case is now type-system-enforced via
// `readOnly: true` on the read schema, and A → B is a non-breaking
// transition if TRA-592 personas show sync workflows are the dominant
// integration shape.
type UpdateAssetRequest struct {
	ExternalKey         *string              `json:"external_key" validate:"omitempty,min=1,max=255"`
	Name                *string              `json:"name" validate:"omitempty,min=1,max=255"`
	Description         *string              `json:"description" validate:"omitempty,max=1024"`
	LocationID          *int                 `json:"location_id" example:"42"`
	LocationExternalKey *string              `json:"location_external_key,omitempty" validate:"omitempty,min=1,max=255" example:"WHS-01"`
	ValidFrom           *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-01-01T00:00:00Z"`
	ValidTo             *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-01-01T00:00:00Z"`
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
	Tags []shared.Tag `json:"tags"`
}

type CreateAssetWithTagsRequest struct {
	CreateAssetRequest
	Tags []shared.TagRequest `json:"tags,omitempty" validate:"omitempty,dive"`
}

type AssetViewListResponse struct {
	Data       []AssetView       `json:"data"`
	Pagination shared.Pagination `json:"pagination"`
}

// AssetWithLocation is AssetView plus the resolved location natural key.
// Populated by GetAssetByExternalKey / list-with-join storage methods;
// returned to HTTP handlers which then project it to PublicAssetView.
// Wire field renamed in TRA-580 C-3.
type AssetWithLocation struct {
	AssetView
	LocationExternalKey *string `json:"location_external_key,omitempty"`
}

// ListFilter carries the optional filters the assets list endpoint supports.
type ListFilter struct {
	// OR semantics within and across LocationIDs / LocationExternalKeys —
	// a row matches if its current location appears in either set.
	LocationIDs          []int
	LocationExternalKeys []string
	// Equality match on a.external_key (any-of). Single value yields the
	// natural-key lookup that lives on the collection per TRA-600.
	ExternalKeys []string
	IsActive     *bool
	Q            *string // substring match (case-insensitive) on name, external_key, description, and active tag values
	Sorts        []ListSort
	Limit        int
	Offset       int
}

// ListSort is one (field, direction) entry.
type ListSort struct {
	Field string
	Desc  bool
}
