package asset

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/org"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/util/httputil"
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
	Name                string               `json:"name" validate:"required,min=1,max=255,no_control_chars"`
	Description         *string              `json:"description,omitempty" validate:"omitempty,min=1,max=1024,no_control_chars"`
	LocationID          *int                 `json:"location_id,omitempty" validate:"omitempty,min=1,max=2147483647" example:"42"`
	LocationExternalKey *string              `json:"location_external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern" example:"WHS-01"`
	ValidFrom           *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-01-01T00:00:00Z"`
	ValidTo             *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-01-01T00:00:00Z"`
	Metadata            map[string]any       `json:"metadata,omitempty"`
	IsActive            *bool                `json:"is_active,omitempty" example:"true"`
}

// PublicReadOnlyFields names the JSON keys on PublicAssetView that the PATCH
// handler silently strips from the request body before strict decoding so a
// verbatim GET → PATCH round-trip succeeds (TRA-608 / BB18 §1.7). Only the
// round-trip-safe, server-owned timestamps and surrogate IDs are on this
// list. Fields not listed here (typos, write-only fields off this resource)
// still produce a 400.
//
// TRA-686 / BB29 F7+F8: `external_key` and `tags` were removed from the
// strip list. They now each have a dedicated reject category on PATCH —
// see PublicRejectPatchFields — because silent-drop hid bugs in
// read-modify-write integrations (the integrator believed the mutation
// took effect; the server quietly ignored it). The strip-on-PATCH rule
// established in TRA-674 was reversed because the visible-failure mode is
// the safer integrator contract.
//
// TRA-681: `location_external_key` is the derived natural-key form for the
// `location_id` FK and is read-only on PATCH — silently stripped along with
// the other server-owned fields. The surrogate `location_id` remains the
// mutable form. Integrators do GET → mutate `location_id` → PATCH back
// with stale `location_external_key` still in the body; the server strips
// it and processes `location_id` unconditionally. The strip is uniform
// regardless of agreement with the surrogate — natural-key on PATCH
// expresses a read, not a write. (Unlike `external_key`, there is no
// rename endpoint for the FK; the natural-key form is fully derivable, so
// echoing the GET-side value back is genuinely a no-op rather than a
// caller-visible mistake.)
//
// Source of truth for the corresponding spec annotations:
// internal/tools/apispec/postprocess.go readOnlyFields["asset.PublicAssetView"]
// (the spec-side readOnly markers are coordinated under TRA-672).
var PublicReadOnlyFields = []string{"id", "created_at", "updated_at", "deleted_at", "location_external_key"}

// UpdateAssetRequest is the PATCH body (RFC 7396 JSON Merge Patch). The handler decodes it via
// DecodeJSONStrictWithNullsTolerant against PublicReadOnlyFields, so
// PublicAssetView's round-trip-safe read-only fields (id, created_at,
// updated_at, deleted_at, location_external_key) are silently stripped on
// a verbatim GET → PATCH round-trip. `external_key` and `tags` are
// pre-decode rejected with 400 instead of silently dropped (TRA-686 /
// BB29 F7+F8) — see PublicRejectPatchFields.
//
// description, location_id, and valid_to all accept JSON null on the wire
// and clear the field server-side (TRA-614 / BB19 §S1). Each null surfaces
// here as a Clear* sentinel set by the handler; the underlying pointer
// remains nil because Go's json decoder treats `null` and "omitted" the
// same on pointer fields.
//
// external_key is intentionally NOT on this struct. It is the natural /
// join key downstream systems rely on; mutating it via a generic PATCH
// would silently disconnect those joins. POST /api/v1/assets/{asset_id}/rename
// is the dedicated path (TRA-664 / BB26 D7). On PATCH, an external_key
// field is rejected with 400 read_only naming the rename endpoint
// (TRA-686 / BB29 F8).
//
// location_external_key is intentionally NOT on this struct (TRA-681).
// The natural-key form is derived from location_id and is read-only on
// PATCH — silently stripped along with the other server-owned fields
// (see PublicReadOnlyFields). To change the location on PATCH, send
// location_id; to clear it, send `"location_id": null`.
type UpdateAssetRequest struct {
	Name        *string              `json:"name" validate:"omitempty,min=1,max=255,no_control_chars"`
	Description *string              `json:"description" validate:"omitempty,min=1,max=1024,no_control_chars"`
	LocationID  *int                 `json:"location_id" validate:"omitempty,min=1,max=2147483647" example:"42"`
	ValidFrom   *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-01-01T00:00:00Z"`
	ValidTo     *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-01-01T00:00:00Z"`
	// Set by the PATCH handler when the body had an explicit `null` for the
	// corresponding read-side-nullable field, to request a column-clear
	// (TRA-614 / TRA-468). Not decoded from JSON directly.
	ClearDescription bool            `json:"-" swaggerignore:"true"`
	ClearLocationID  bool            `json:"-" swaggerignore:"true"`
	ClearValidTo     bool            `json:"-" swaggerignore:"true"`
	Metadata         *map[string]any `json:"metadata"`
	IsActive         *bool           `json:"is_active"`
}

// PublicRejectPatchFields names the JSON keys that PATCH /api/v1/assets/{id}
// rejects pre-decode with 400 validation_error. Two categories:
//
//   - `tags` is managed via the dedicated /assets/{asset_id}/tags
//     subresource (POST + DELETE). On PATCH a `tags` body field is rejected
//     with code=invalid_value pointing at the subresource endpoints. The
//     silent-drop alternative hid bugs in read-modify-write integrations
//     where callers reused the GET body shape — the asset's tag set looked
//     unchanged, the server quietly ignored the write.
//   - `external_key` is mutated through POST /assets/{asset_id}/rename. On
//     PATCH it is rejected with code=read_only pointing at the rename
//     endpoint, mirroring the rationale: a "rename via PATCH" silently
//     dropped is a much worse failure mode than an explicit 400.
//
// Source: TRA-686 / BB29 F7+F8.
var PublicRejectPatchFields = map[string]httputil.FieldRejectPolicy{
	"tags": {
		Code:    "invalid_value",
		Message: "tags are managed via POST /api/v1/assets/{asset_id}/tags and DELETE /api/v1/assets/{asset_id}/tags/{tag_id}",
	},
	"external_key": {
		Code:    "read_only",
		Message: "external_key is immutable via PATCH; use POST /api/v1/assets/{asset_id}/rename",
	},
}

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
