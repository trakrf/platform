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
	ExternalKey         string               `json:"external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern"`
	Name                string               `json:"name" validate:"required,min=1,max=255"`
	Description         string               `json:"description,omitempty" validate:"omitempty,max=1024"`
	LocationID          *int                 `json:"location_id,omitempty" example:"42"`
	LocationExternalKey *string              `json:"location_external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern" example:"WHS-01"`
	ValidFrom           *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-01-01T00:00:00Z"`
	ValidTo             *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-01-01T00:00:00Z"`
	Metadata            any                  `json:"metadata,omitempty"`
	IsActive            *bool                `json:"is_active,omitempty" example:"true"`
}

// PublicReadOnlyFields names the JSON keys on PublicAssetView that the PATCH
// handler strips from the request body before strict decoding so a verbatim
// GET → PATCH round-trip succeeds (TRA-608 / BB18 §1.7). Fields not listed
// here (typos, write-only fields off this resource) still produce a 400.
//
// TRA-674 / BB27 F3: `external_key` and `tags` were moved onto the strip list
// to make full-object PATCH the supported integrator idiom — mirrors what
// Stripe does. Mutating either still has a dedicated path
// (POST /assets/{asset_id}/rename, POST/DELETE /assets/{asset_id}/tags); the
// PATCH body just silently ignores them, the same way it silently ignores
// `id` / `created_at` / `updated_at` / `asset_deleted_at`. The previous
// rejection-based behavior (TRA-643 for `tags`, TRA-664 for `external_key`)
// forced every code-generated client to write a strip-on-PATCH helper, so
// the rule is reversed pre-launch in favor of the more generator-friendly
// shape.
//
// Source of truth for the corresponding spec annotations:
// internal/tools/apispec/postprocess.go readOnlyFields["asset.PublicAssetView"]
// (the spec-side readOnly markers are coordinated under TRA-672).
var PublicReadOnlyFields = []string{"id", "created_at", "updated_at", "asset_deleted_at", "external_key", "tags"}

// UpdateAssetRequest is the PATCH body (RFC 7396 JSON Merge Patch). The handler decodes it via
// DecodeJSONStrictWithNullsTolerant against PublicReadOnlyFields, so
// PublicAssetView's round-trip-safe read-only fields (id, created_at,
// updated_at) are silently ignored on a verbatim GET → PATCH round-trip
// while any other unknown field — including `tags`, which is managed via
// the /assets/{id}/tags subresource — still produces a 400.
//
// description, location_id, location_external_key, and valid_to all accept
// JSON null on the wire and clear the field server-side (TRA-614 / BB19
// §S1). Each null surfaces here as a Clear* sentinel set by the handler;
// the underlying pointer remains nil because Go's json decoder treats
// `null` and "omitted" the same on pointer fields.
//
// external_key is intentionally NOT on this struct. It is the natural /
// join key downstream systems rely on; mutating it via a generic PATCH
// would silently disconnect those joins. POST /api/v1/assets/{asset_id}/rename
// is the dedicated path (TRA-664 / BB26 D7). On PATCH, an external_key
// field is silently stripped along with other read-only fields per
// TRA-674 / BB27 F3 — see PublicReadOnlyFields.
type UpdateAssetRequest struct {
	Name                *string              `json:"name" validate:"omitempty,min=1,max=255"`
	Description         *string              `json:"description" validate:"omitempty,max=1024"`
	LocationID          *int                 `json:"location_id" example:"42"`
	LocationExternalKey *string              `json:"location_external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern" example:"WHS-01"`
	ValidFrom           *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-01-01T00:00:00Z"`
	ValidTo             *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-01-01T00:00:00Z"`
	// Set by the PATCH handler when the body had an explicit `null` for the
	// corresponding read-side-nullable field, to request a column-clear
	// (TRA-614 / TRA-468). Not decoded from JSON directly.
	ClearDescription bool  `json:"-" swaggerignore:"true"`
	ClearLocationID  bool  `json:"-" swaggerignore:"true"`
	ClearValidTo     bool  `json:"-" swaggerignore:"true"`
	Metadata         *any  `json:"metadata"`
	IsActive         *bool `json:"is_active"`
}

// PublicImmutablePatchFields maps the JSON keys that PATCH /api/v1/assets/{id}
// must reject to the dedicated operation that can mutate them.
//
// TRA-674 / BB27 F3: previously contained `external_key`, but the strip-on-
// PATCH rule now applies (see PublicReadOnlyFields). Kept as the registration
// point for any future field that genuinely needs a hard rejection (a field
// where silent strip would be confusing or unsafe) rather than the
// strip-and-ignore default. Empty map means RejectImmutableFields is a no-op
// for assets.
var PublicImmutablePatchFields = map[string]string{}

// RenameAssetRequest is the body of POST /api/v1/assets/{asset_id}/rename
// (TRA-664 / BB26 D7). The dedicated operation makes external_key mutation
// explicit and discoverable in URL surfaces, audit logs, and generated
// client method names.
type RenameAssetRequest struct {
	ExternalKey string `json:"external_key" validate:"required,min=1,max=255,external_key_pattern" example:"ASSET-0042"`
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
	// IncludeDeleted relaxes the default a.deleted_at IS NULL filter so
	// soft-deleted rows are returned alongside live rows. Orthogonal to
	// IsActive (TRA-659 / BB25 A3). Temporal validity still applies.
	IncludeDeleted bool
	Sorts          []ListSort
	Limit          int
	Offset         int
}

// ListSort is one (field, direction) entry.
type ListSort struct {
	Field string
	Desc  bool
}
