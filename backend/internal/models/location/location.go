package location

import (
	"time"

	"github.com/trakrf/platform/backend/internal/models/org"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

type Location struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	OrgID       int        `json:"org_id"`
	Org         *org.Org   `json:"org,omitempty"`
	ExternalKey string     `json:"external_key" validate:"required,min=1,max=255"`
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
	Name string `json:"name" validate:"required,min=1,max=255,no_control_chars" example:"Warehouse 1"`
	// external_key is optional. Omit to receive a server-assigned key in the
	// format LOC-NNNN (per-organization sequence), parallel to assets'
	// ASSET-NNNN behavior. When supplied, must satisfy the external_key_pattern.
	ExternalKey       string               `json:"external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern" example:"wh1"`
	ParentID          *int                 `json:"parent_id,omitempty" validate:"omitempty,min=1,max=2147483647" example:"42"`
	ParentExternalKey *string              `json:"parent_external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern" example:"wh1"`
	Description       *string              `json:"description,omitempty" validate:"omitempty,min=1,max=1024,no_control_chars" example:"Main warehouse location"`
	ValidFrom         *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-12-14T00:00:00Z"`
	ValidTo           *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14T00:00:00Z"`
	IsActive          *bool                `json:"is_active,omitempty" example:"true"`
}

// PublicReadOnlyFields names the JSON keys on PublicLocationView that the
// PATCH handler silently strips from the request body before strict
// decoding so a verbatim GET → PATCH round-trip succeeds (TRA-608 / BB18
// §1.7). Only the round-trip-safe, server-owned timestamps and surrogate
// IDs are on this list.
//
// TRA-686 / BB29 F7+F8: `external_key`, `tags`, and `parent_external_key`
// were removed from the strip list. The first two each have a dedicated
// reject category on PATCH — see PublicRejectPatchFields — because
// silent-drop hid bugs in read-modify-write integrations. The third
// (parent_external_key) remains on the rename-managed reject list
// alongside `external_key`: on locations both natural-key forms are
// rooted in a renameable column on a different row, so the rename
// endpoint is the only valid mutation path.
//
// Source of truth for the corresponding spec annotations:
// internal/tools/apispec/postprocess.go readOnlyFields["location.PublicLocationView"]
// (the spec-side readOnly markers are coordinated under TRA-672).
var PublicReadOnlyFields = []string{"id", "created_at", "updated_at", "deleted_at"}

// UpdateLocationRequest is the PATCH body (RFC 7396 JSON Merge Patch). The handler decodes it via
// DecodeJSONStrictWithNullsTolerant against PublicReadOnlyFields, so
// PublicLocationView's round-trip-safe read-only fields (id, created_at,
// updated_at, deleted_at) are silently stripped on a verbatim GET → PATCH
// round-trip. `external_key`, `tags`, and `parent_external_key` are
// pre-decode rejected with 400 instead of silently dropped (TRA-686 /
// BB29 F7+F8) — see PublicRejectPatchFields.
//
// description, parent_id, and valid_to all accept JSON null on the wire
// and clear the field server-side (TRA-614 / BB19 §S1). Each null surfaces
// here as a Clear* sentinel set by the handler; the underlying pointer
// remains nil because Go's json decoder treats `null` and "omitted" the
// same on pointer fields.
//
// external_key and parent_external_key are intentionally NOT on this
// struct. They are natural-key fields rooted in renameable columns —
// mutating them via a generic PATCH would silently disconnect downstream
// joins. POST /api/v1/locations/{location_id}/rename is the dedicated
// path (TRA-664 / BB26 D7). On PATCH both forms are rejected with 400
// read_only naming the rename endpoint (TRA-686 / BB29 F8).
type UpdateLocationRequest struct {
	Name        *string              `json:"name,omitempty" validate:"omitempty,min=1,max=255,no_control_chars" example:"Warehouse 1"`
	ParentID    *int                 `json:"parent_id,omitempty" validate:"omitempty,min=1,max=2147483647" example:"42"`
	Description *string              `json:"description,omitempty" validate:"omitempty,min=1,max=1024,no_control_chars" example:"Updated description"`
	ValidFrom   *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-12-14T00:00:00Z"`
	ValidTo     *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14T00:00:00Z"`
	// Set by the PATCH handler when the body had an explicit `null` for the
	// corresponding read-side-nullable field, to request a column-clear
	// (TRA-614 / TRA-468). Not decoded from JSON directly.
	ClearDescription bool  `json:"-" swaggerignore:"true"`
	ClearParentID    bool  `json:"-" swaggerignore:"true"`
	ClearValidTo     bool  `json:"-" swaggerignore:"true"`
	IsActive         *bool `json:"is_active,omitempty" example:"true"`
}

// PublicRejectPatchFields names the JSON keys that PATCH
// /api/v1/locations/{id} rejects pre-decode with 400 validation_error.
// Three fields, two categories — see asset.PublicRejectPatchFields for
// the underlying rationale (silent-drop on read-modify-write is the
// failure mode this prevents).
//
//   - `tags` → invalid_value, points at POST/DELETE
//     /locations/{location_id}/tags.
//   - `external_key` and `parent_external_key` → read_only, both point at
//     POST /locations/{location_id}/rename. parent_external_key is the
//     natural-key form of the parent_id FK; mutating it on PATCH would
//     either be a no-op (if it agrees with parent_id, redundant) or a
//     write to the wrong row (the parent's natural key is owned by the
//     parent location, not this one), so the only legal mutation path is
//     the rename endpoint on the parent row.
//
// Source: TRA-686 / BB29 F7+F8.
var PublicRejectPatchFields = map[string]httputil.FieldRejectPolicy{
	"tags": {
		Code:    "invalid_value",
		Message: "tags are managed via POST /api/v1/locations/{location_id}/tags and DELETE /api/v1/locations/{location_id}/tags/{tag_id}",
	},
	"external_key": {
		Code:    "read_only",
		Message: "external_key is immutable via PATCH; use POST /api/v1/locations/{location_id}/rename",
	},
	"parent_external_key": {
		Code:    "read_only",
		Message: "parent_external_key is immutable via PATCH; rename the parent location with POST /api/v1/locations/{location_id}/rename, or re-parent this row by sending `parent_id`",
	},
}

// RenameLocationRequest is the body of POST /api/v1/locations/{location_id}/rename
// (TRA-664 / BB26 D7). The dedicated operation makes external_key mutation
// explicit and is distinct from a generic PATCH in audit logs (different
// URL surface). The response includes `descendant_count_affected` so
// integrators can decide whether to re-fetch the subtree even though the
// rename only mutates this row's own natural key.
type RenameLocationRequest struct {
	ExternalKey string `json:"external_key" validate:"required,min=1,max=255,external_key_pattern" example:"wh1"`
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
	// IncludeDeleted relaxes the default l.deleted_at IS NULL filter so
	// soft-deleted rows are returned alongside live rows. Orthogonal to
	// IsActive (TRA-659 / BB25 A3). Temporal validity still applies.
	IncludeDeleted bool
	Sorts          []ListSort
	Limit          int
	Offset         int
}

type ListSort struct {
	Field string
	Desc  bool
}
