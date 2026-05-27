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
	Name string `json:"name" validate:"required,min=1,max=255,display_name" example:"Warehouse 1"`
	// external_key is optional. Omit to receive a server-assigned key in the
	// format LOC-NNN (per-organization sequence, 3-digit zero-pad), parallel
	// to (but narrower than) assets' ASSET-NNNN behavior. When supplied,
	// must satisfy the external_key_pattern.
	ExternalKey       string               `json:"external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern" example:"wh1"`
	ParentID          *int                 `json:"parent_id,omitempty" validate:"omitempty,min=1" example:"42"`
	ParentExternalKey *string              `json:"parent_external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern" example:"wh1"`
	Description       *string              `json:"description,omitempty" validate:"omitempty,min=1,max=1024,no_control_chars" example:"Main warehouse location"`
	ValidFrom         *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-12-14T00:00:00Z"`
	ValidTo           *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14T00:00:00Z"`
	IsActive          *bool                `json:"is_active,omitempty" example:"true"`
}

// PublicReadOnlyFields names the JSON keys on PublicLocationView that the
// PATCH handler drops from the request body before strict decoding so the
// strict-decode unknown-field check does not trip on them.
//
// TRA-710 (BB33 F2): the four server-managed fields (id, created_at,
// updated_at, deleted_at) are policed by the post-decode echo check in
// the PATCH handler — verbatim GET → PATCH matches and is normalized out
// (silent strip), while a value differing from current state returns 400
// with code=read_only. `tags` also moved onto the echo check under the
// same rule (off PublicRejectPatchFields).
//
// TRA-699 (BB31 §2): `external_key` and `parent_external_key` are NOT on
// this list. They are policed by the same post-decode echo check via
// dedicated decode targets.
//
// `parent_id` (the surrogate form of the parent reference) remains fully
// writable on PATCH — only the natural-key form is locked down.
//
// Source of truth for the corresponding spec annotations:
// internal/tools/apispec/postprocess.go readOnlyFields["location.PublicLocationView"]
// (the spec-side readOnly markers are coordinated under TRA-672).
var PublicReadOnlyFields = []string{"id", "created_at", "updated_at", "deleted_at"}

// UpdateLocationRequest is the PATCH body (RFC 7396 JSON Merge Patch). The handler decodes it via
// DecodeJSONStrictWithNullsTolerant against PublicReadOnlyFields. TRA-710
// (BB33 F2): `id`, `created_at`, `updated_at`, `deleted_at`, and `tags`
// are dropped from the decode and policed by the post-decode echo check —
// matching the current resource value is silently normalized out, a
// differing value returns 400 with code=read_only.
//
// description, parent_id, parent_external_key, and valid_to all accept JSON null on the wire
// and clear the field server-side (TRA-614 / BB19 §S1). Each null surfaces
// here as a Clear* sentinel set by the handler; the underlying pointer
// remains nil because Go's json decoder treats `null` and "omitted" the
// same on pointer fields.
//
// TRA-699 (BB31 §2): `external_key` is decoded into a dedicated pointer
// but policed by the post-decode echo check in the PATCH handler — accept
// the value if it matches the current resource state (silent no-op);
// reject if it differs (400 read_only naming POST /locations/{id}/rename).
// It carries no validation tag because the value is never written by
// PATCH; the handler nils it out after the echo check passes.
//
// TRA-719 / BB35 B2: `parent_external_key` is now WRITABLE on PATCH,
// symmetric with CreateLocationRequest. The handler dispatches it through
// the same FK resolution logic used at create time (resolveParent), so a
// re-parent via the natural key works without requiring integrators to
// resolve the surrogate first.
//
// TRA-757 (BB50/51/52 F1): `parent_id` and `parent_external_key` may be
// supplied together when they name the same parent (silently normalized
// to a single re-parent operation); only a disagreement returns 400
// ambiguous_fields. Symmetric with CreateLocationRequest. See the PATCH
// reconciliation block in handlers/locations.
type UpdateLocationRequest struct {
	Name        *string              `json:"name,omitempty" validate:"omitempty,min=1,max=255,display_name" example:"Warehouse 1"`
	ParentID    *int                 `json:"parent_id,omitempty" validate:"omitempty,min=1" example:"42"`
	Description *string              `json:"description,omitempty" validate:"omitempty,min=1,max=1024,no_control_chars" example:"Updated description"`
	ValidFrom   *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-12-14T00:00:00Z"`
	ValidTo     *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14T00:00:00Z"`
	// TRA-699 external_key echo field — decoded so the handler can compare
	// against current state, then nilled before storage update.
	ExternalKey       *string `json:"external_key" swaggerignore:"true"`
	ParentExternalKey *string `json:"parent_external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern" example:"wh1"`
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
//
// TRA-710 (BB33 F2): `tags` moved off this map. It is now policed by the
// post-decode echo check in the PATCH handler under the uniform
// accept-if-matches, reject-if-differs rule.
//
// TRA-699 (BB31 §2): `external_key` and `parent_external_key` moved off
// this map under the same rule.
//
// Kept exported as a (currently empty) map so the handler call site
// remains compile-stable.
var PublicRejectPatchFields = map[string]httputil.FieldRejectPolicy{}

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
