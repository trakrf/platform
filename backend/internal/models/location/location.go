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
	ExternalKey       string               `json:"external_key" validate:"required,min=1,max=255,external_key_pattern" example:"wh1"`
	ParentID          *int                 `json:"parent_id,omitempty" validate:"omitempty,min=1" example:"42"`
	ParentExternalKey *string              `json:"parent_external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern" example:"wh1"`
	Description       string               `json:"description,omitempty" validate:"omitempty,max=1024" example:"Main warehouse location"`
	ValidFrom         *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-12-14T00:00:00Z"`
	ValidTo           *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14T00:00:00Z"`
	IsActive          *bool                `json:"is_active,omitempty" example:"true"`
}

// PublicReadOnlyFields names the JSON keys on PublicLocationView that are
// server-owned and round-trip safe: the PATCH handler strips them from the
// request body before strict decoding so a verbatim GET → PATCH round-trip
// succeeds (TRA-608 / BB18 §1.7).
//
// `tags` is intentionally NOT on this list. Tags are managed via the
// /locations/{id}/tags subresource, so a `tags` key in a PATCH body is a
// caller-side mistake worth surfacing. Strict decode rejects it as an
// unknown field with code=invalid_value, matching unknown-field behavior
// (TRA-643 / BB22 F1).
//
// Source of truth for the corresponding spec annotations:
// internal/tools/apispec/postprocess.go readOnlyFields["location.PublicLocationView"].
var PublicReadOnlyFields = []string{"id", "created_at", "updated_at", "tree_path", "depth", "location_deleted_at"}

// UpdateLocationRequest is the PATCH body (RFC 7396 JSON Merge Patch). The handler decodes it via
// DecodeJSONStrictWithNullsTolerant against PublicReadOnlyFields, so
// PublicLocationView's round-trip-safe read-only fields (id, created_at,
// updated_at, tree_path, depth) are silently ignored on a verbatim GET →
// PUT round-trip while any other unknown field — including `tags`, which
// is managed via the /locations/{id}/tags subresource — still produces a
// 400.
//
// description, parent_id, parent_external_key, and valid_to all accept
// JSON null on the wire and clear the field server-side (TRA-614 / BB19
// §S1). Each null surfaces here as a Clear* sentinel set by the handler;
// the underlying pointer remains nil because Go's json decoder treats
// `null` and "omitted" the same on pointer fields.
//
// external_key is intentionally NOT on this struct (TRA-664 / BB26 D7). It
// is the natural / join key downstream systems rely on and is the
// canonical source for tree_path; mutating it via a generic PATCH would
// silently disconnect joins and cascade tree_path changes without notice.
// The handler rejects any external_key in the PATCH body with
// code=immutable_field and a pointer to
// POST /api/v1/locations/{location_id}/rename.
type UpdateLocationRequest struct {
	Name              *string              `json:"name,omitempty" validate:"omitempty,min=1,max=255" example:"Warehouse 1"`
	ParentID          *int                 `json:"parent_id,omitempty" validate:"omitempty,min=1" example:"42"`
	ParentExternalKey *string              `json:"parent_external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern" example:"wh1"`
	Description       *string              `json:"description,omitempty" validate:"omitempty,max=1024" example:"Updated description"`
	ValidFrom         *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-12-14T00:00:00Z"`
	ValidTo           *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-12-14T00:00:00Z"`
	// Set by the PATCH handler when the body had an explicit `null` for the
	// corresponding read-side-nullable field, to request a column-clear
	// (TRA-614 / TRA-468). Not decoded from JSON directly.
	ClearDescription bool  `json:"-" swaggerignore:"true"`
	ClearParentID    bool  `json:"-" swaggerignore:"true"`
	ClearValidTo     bool  `json:"-" swaggerignore:"true"`
	IsActive         *bool `json:"is_active,omitempty" example:"true"`
}

// PublicImmutablePatchFields maps the JSON keys that PATCH /api/v1/locations/{id}
// must reject to the dedicated operation that can mutate them. Source of
// truth for the handler's RejectImmutableFields call; mirrors the
// UpdateLocationRequest schema's deliberate omission of these keys. TRA-664.
var PublicImmutablePatchFields = map[string]string{
	"external_key": "POST /api/v1/locations/{location_id}/rename",
}

// RenameLocationRequest is the body of POST /api/v1/locations/{location_id}/rename
// (TRA-664 / BB26 D7). The dedicated operation makes external_key mutation
// explicit and consolidates the tree_path cascade (this row + all
// descendants) in one place so the response can carry the
// descendant_count_affected signal.
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
