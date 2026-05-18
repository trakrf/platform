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

// TRA-734 (BB40 F3): location_id and location_external_key are no longer
// on CreateAssetRequest. Asset location is scan/operational data — collected
// through the ingestion paths (fixed-reader MQTT pipeline + handheld UI
// submission), never set through the public API. The handler rejects the
// fields pre-decode with code=read_only naming the consumption paths
// (GET /assets/{id}, GET /assets/{id}/history, GET /reports/asset-locations).
// See PublicRejectCreateFields in the assets handler.
type CreateAssetRequest struct {
	OrgID int `json:"-" swaggerignore:"true"`
	// external_key is optional. Omit to receive a server-assigned key in the
	// format ASSET-NNNN (per-organization sequence). When supplied, must
	// satisfy the external_key_pattern.
	ExternalKey string               `json:"external_key,omitempty" validate:"omitempty,min=1,max=255,external_key_pattern" example:"forklift-3"`
	Name        string               `json:"name" validate:"required,min=1,max=255,display_name" example:"Forklift 3"`
	Description *string              `json:"description,omitempty" validate:"omitempty,min=1,max=1024,no_control_chars" example:"Main warehouse forklift"`
	ValidFrom   *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-01-01T00:00:00Z"`
	ValidTo     *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-01-01T00:00:00Z"`
	Metadata    map[string]any       `json:"metadata,omitempty"`
	IsActive    *bool                `json:"is_active,omitempty" example:"true"`
}

// PublicReadOnlyFields names the JSON keys on PublicAssetView that the PATCH
// handler drops from the request body before strict decoding so the
// strict-decode unknown-field check does not trip on them.
//
// TRA-710 (BB33 F2): the four server-managed fields on this list (id,
// created_at, updated_at, deleted_at) are policed by the post-decode echo
// check in the PATCH handler under the uniform accept-if-matches,
// reject-if-differs rule. Same shape as the natural-key reference fields
// (TRA-699): a verbatim GET → PATCH round-trip succeeds (the submitted
// value matches current; the field is normalized out), while a value
// differing from the current resource state returns 400 with code=read_only.
// Pre-TRA-710 these four were silent-stripped regardless of value, which
// hid bugs where integrators tried to mutate id / created_at by setting
// it explicitly. The strip-from-decode step remains because the wire
// fields are not decoded into UpdateAssetRequest at all (no Go destination
// for them); the echo check fires in addition, against the raw body.
//
// TRA-699 (BB31 §2): `external_key`, `location_external_key`, and
// `location_id` are NOT on this list. They are policed by the same
// post-decode echo check via dedicated decode targets.
//
// TRA-710 also moved `tags` off PublicRejectPatchFields onto the echo
// check (same rule). `tags` is not in this list because it is decoded
// directly into PublicAssetView.Tags-style structures on the read path
// only; the handler captures the raw body value separately.
//
// Source of truth for the corresponding spec annotations:
// internal/tools/apispec/postprocess.go readOnlyFields["asset.PublicAssetView"]
// (the spec-side readOnly markers are coordinated under TRA-672).
var PublicReadOnlyFields = []string{"id", "created_at", "updated_at", "deleted_at"}

// UpdateAssetRequest is the PATCH body (RFC 7396 JSON Merge Patch). The handler decodes it via
// DecodeJSONStrictWithNullsTolerant against PublicReadOnlyFields. TRA-710
// (BB33 F2): `id`, `created_at`, `updated_at`, `deleted_at`, and `tags`
// are dropped from the decode and policed by the post-decode echo check —
// matching the current resource value is silently normalized out, a
// differing value returns 400 with code=read_only.
//
// description and valid_to accept JSON null on the wire and clear the
// field server-side (TRA-614 / BB19 §S1). Each null surfaces here as a
// Clear* sentinel set by the handler; the underlying pointer remains nil
// because Go's json decoder treats `null` and "omitted" the same on
// pointer fields.
//
// TRA-699 (BB31 §2): three natural-key reference fields are decoded into
// dedicated pointers but policed by the post-decode echo check in the
// PATCH handler — accept the value if it matches the current resource
// state (silent no-op); reject if it differs (400 read_only naming the
// proper write path). Fields:
//   - ExternalKey (own natural key) → POST /assets/{id}/rename
//   - LocationExternalKey, LocationID (TRA-734 / BB40 F3 — scan/operational
//     data, collected through ingestion paths, never settable on the public
//     API)
//
// None of these three fields carry validation tags because the value is
// never written by PATCH; the only valid use is to echo the current value
// back. The handler nils them out after the echo check passes.
type UpdateAssetRequest struct {
	Name        *string              `json:"name" validate:"omitempty,min=1,max=255,display_name" example:"Forklift 3"`
	Description *string              `json:"description" validate:"omitempty,min=1,max=1024,no_control_chars" example:"Updated description"`
	ValidFrom   *shared.FlexibleDate `json:"valid_from,omitempty" swaggertype:"string" example:"2025-01-01T00:00:00Z"`
	ValidTo     *shared.FlexibleDate `json:"valid_to,omitempty" swaggertype:"string" example:"2026-01-01T00:00:00Z"`
	// TRA-699 natural-key echo fields. Decoded so the handler can compare
	// against current state, then nilled before storage update.
	ExternalKey         *string `json:"external_key" swaggerignore:"true"`
	LocationID          *int    `json:"location_id" swaggerignore:"true"`
	LocationExternalKey *string `json:"location_external_key" swaggerignore:"true"`
	// Set by the PATCH handler when the body had an explicit `null` for the
	// corresponding read-side-nullable field, to request a column-clear
	// (TRA-614 / TRA-468). Not decoded from JSON directly.
	ClearDescription bool            `json:"-" swaggerignore:"true"`
	ClearValidTo     bool            `json:"-" swaggerignore:"true"`
	Metadata         *map[string]any `json:"metadata"`
	IsActive         *bool           `json:"is_active" example:"true"`
}

// PublicRejectPatchFields names the JSON keys that PATCH /api/v1/assets/{id}
// rejects pre-decode with 400 validation_error.
//
// TRA-710 (BB33 F2): `tags` moved off this map. It is now policed by the
// post-decode echo check in the PATCH handler under the uniform
// accept-if-matches, reject-if-differs rule (same as the natural-key
// reference fields). Same end-state for a caller who tries to mutate
// (400 with read_only naming the subresource); the new shape additionally
// accepts a verbatim GET → PATCH echo as a silent no-op rather than
// 400-ing it.
//
// TRA-699 (BB31 §2): `external_key` moved off this map under the same
// rule.
//
// Kept exported as a (currently empty) map so existing handler call sites
// remain compile-stable; future fields that need pre-decode reject (i.e.
// fields whose mere presence is invalid regardless of value) can be added
// here.
var PublicRejectPatchFields = map[string]httputil.FieldRejectPolicy{}

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
