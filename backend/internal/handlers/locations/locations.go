package locations

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	httputil.RegisterCustomValidations(v)
	return v
}()

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{
		storage: storage,
	}
}

// resolveParent reconciles the parent_id (canonical surrogate) and
// parent_external_key (natural-key alternate) inputs on Create. Exactly
// one is expected to be set (TRA-681 oneOf constraint enforced upstream
// in the Create handler via the ambiguous_fields pre-check). PATCH never
// supplies the natural-key form — it is stripped before this function
// runs (see location.PublicReadOnlyFields).
//
// Both nil → nil (no parent). When parent_id is set it is used; otherwise
// parent_external_key is resolved via lookup.
//
// TRA-674 / BB27 F2 / TRA-681: a nonexistent surrogate `parent_id`
// returns the same envelope shape as a nonexistent natural-key
// `parent_external_key` — both surface keyed on the offending field as
// 400 validation_error with code=fk_not_found. Previously the surrogate
// path reached the storage layer and tripped the FK constraint,
// surfacing as 500 internal_error.
func (handler *Handler) resolveParent(
	r *http.Request, orgID int, parentID *int, parentExternalKey *string,
) (*int, *modelerrors.FieldError) {
	hasID := parentID != nil
	hasExt := parentExternalKey != nil && *parentExternalKey != ""

	if !hasID && !hasExt {
		return nil, nil
	}
	if hasID {
		parent, err := handler.storage.GetLocationByID(r.Context(), orgID, *parentID)
		if err != nil {
			return nil, &modelerrors.FieldError{
				Field:   "parent_id",
				Code:    "internal_error",
				Message: err.Error(),
			}
		}
		if parent == nil {
			return nil, &modelerrors.FieldError{
				Field:   "parent_id",
				Code:    "fk_not_found",
				Message: fmt.Sprintf("parent_id %d not found", *parentID),
			}
		}
		return parentID, nil
	}

	parent, err := handler.storage.GetLocationByExternalKey(r.Context(), orgID, *parentExternalKey)
	if err != nil {
		return nil, &modelerrors.FieldError{
			Field:   "parent_external_key",
			Code:    "internal_error",
			Message: err.Error(),
		}
	}
	if parent == nil {
		return nil, &modelerrors.FieldError{
			Field:   "parent_external_key",
			Code:    "fk_not_found",
			Message: fmt.Sprintf("parent_external_key %q not found", *parentExternalKey),
		}
	}
	return &parent.ID, nil
}

// @Summary      Create a location
// @Description  Create a new location in the hierarchy, optionally with one or more tags.
// @Description  Set parent_id (canonical) or parent_external_key (alternate) to nest under an existing parent.
// @Description
// @Description  The `external_key` field is optional. Provide a value from your system of record
// @Description  (ERP, WMS, layout/plan) for natural-key joins, or omit it to receive a
// @Description  server-assigned external_key in the format `LOC-NNNN` (per-organization sequence).
// @Description  A caller-supplied external_key that collides with an existing location returns 409.
// @Tags         locations,public
// @ID           locations.create
// @Accept       json
// @Produce      json
// @Param        request  body  location.CreateLocationWithTagsRequest  true  "Location to create with optional tags"
// @Success      201  {object}  locations.CreateLocationResponse
// @Header       201  {string}  Location  "Canonical URL of the created resource"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      415  {object}  modelerrors.ErrorResponse     "unsupported_media_type"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     BearerAuth[locations:write]
// @Router       /api/v1/locations [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	var request location.CreateLocationWithTagsRequest
	// TRA-692: capture both presence and explicit-null sets so the validation
	// envelope can promote omitted/null required fields back to code=required
	// (per §1.2). Drop list is nil — Create has no read-only fields to strip.
	explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(r, &request, nil)
	if err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	// TRA-705 (BB32 §C6): valid_from and is_active are non-nullable on
	// both Create and Update. Omission already means "use server default"
	// — accepting null on Create only (the prior TRA-675 "null-as-now"
	// carve-out) muddied the semantics. Reject explicit null with
	// invalid_value; accumulate every violation (BB32 §D3 pattern).
	var nullViolations []modelerrors.FieldError
	for _, f := range []string{"valid_from", "is_active"} {
		if _, ok := explicitNulls[f]; ok {
			nullViolations = append(nullViolations, modelerrors.FieldError{
				Field:   f,
				Code:    "invalid_value",
				Message: fmt.Sprintf("%s cannot be null; omit the field to use the server default, or provide a value", f),
			})
		}
	}
	if len(nullViolations) > 0 {
		httputil.WriteValidationError(w, r, requestID, nullViolations)
		return
	}

	// TRA-665 / BB26 D3: external_key is optional only by *omission* — an
	// absent key triggers the server auto-mint of LOC-NNNN. When the caller
	// sends `external_key` explicitly, it must validate (min=1 + pattern)
	// to match the PATCH validator on RenameLocationRequest.external_key.
	// An explicit empty string returns 400 too_short; whitespace-only fails
	// the pattern check with 400 invalid_value. The struct field is non-pointer
	// so encoding/json cannot distinguish absent from explicit-empty on its own
	// — presentKeys carries that signal.
	if _, present := presentKeys["external_key"]; present {
		type extKeyCheck struct {
			ExternalKey string `json:"external_key" validate:"min=1,max=255,external_key_pattern"`
		}
		if err := validate.Struct(extKeyCheck{ExternalKey: request.ExternalKey}); err != nil {
			httputil.RespondValidationErrorWithPresence(w, r, err, requestID, presentKeys, explicitNulls)
			return
		}
	}

	// TRA-681: parent_id and parent_external_key form a oneOf on Create
	// bodies — the spec encodes `not: {required: [parent_id,
	// parent_external_key]}` on CreateLocationWithTagsRequest. Reject both-
	// supplied at the handler so callers get a typed ambiguous_fields code
	// they can branch on, rather than relying on a silent server pick.
	_, hasParentID := presentKeys["parent_id"]
	_, hasParentExt := presentKeys["parent_external_key"]
	if hasParentID && hasParentExt {
		httputil.WriteValidationError(w, r, requestID, []modelerrors.FieldError{
			{Field: "parent_id", Code: "ambiguous_fields", Message: "parent_id and parent_external_key are mutually exclusive; supply exactly one"},
			{Field: "parent_external_key", Code: "ambiguous_fields", Message: "parent_id and parent_external_key are mutually exclusive; supply exactly one"},
		})
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationErrorWithPresence(w, r, err, requestID, presentKeys, explicitNulls)
		return
	}

	resolved, fErr := handler.resolveParent(r, orgID, request.ParentID, request.ParentExternalKey)
	if fErr != nil {
		if fErr.Code == "internal_error" {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
				fErr.Message, requestID)

			return
		}
		// TRA-681: fk_not_found is 400 validation_error (was 409 conflict
		// under TRA-678). See assets.Create for rationale.
		httputil.WriteValidationError(w, r, requestID, []modelerrors.FieldError{*fErr})

		return
	}
	request.ParentID = resolved

	if request.IsActive == nil {
		t := true
		request.IsActive = &t
	}
	// valid_from defaults to now when the field is *omitted*. After
	// TRA-649 the parser rejects empty strings and zero-time literals as
	// validation errors, so the handler no longer has to translate
	// silently-substituted zero values into the default.
	if request.ValidFrom == nil {
		fd := shared.FlexibleDate{Time: time.Now().UTC()}
		request.ValidFrom = &fd
	}

	result, err := handler.storage.CreateLocationWithTags(r.Context(), orgID, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				err.Error(), requestID)

			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.Header().Set("Location", "/api/v1/locations/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": location.ToPublicLocationView(*result)})
}

// @Summary      Update a location
// @Description  Apply a JSON Merge Patch (RFC 7396) to a location. Only fields included in the request body are changed; fields set to `null` clear the corresponding nullable column. Omitted fields are left unchanged. An empty body (`{}`) is a no-op and returns the current resource unchanged. Round-trip-safe server-owned fields (`id`, `created_at`, `updated_at`, `deleted_at`) are silently stripped from the body. The natural-key reference fields `external_key` and `parent_external_key` are read-only on PATCH: a value matching the current resource state is silently stripped (so a verbatim GET → PATCH round-trip succeeds), and a differing value returns 400 with `code: read_only` naming the correct write path. To re-parent via PATCH send `parent_id` (surrogate; `null` clears the FK); to re-parent via natural key use POST /locations/{location_id}/rename. Mutate `external_key` via POST /locations/{location_id}/rename; mutate `tags` via POST /locations/{location_id}/tags and DELETE /locations/{location_id}/tags/{tag_id}.
// @Tags         locations,public
// @ID           locations.update
// @Accept       json
// @Produce      json
// @Param        location_id path  int                              true  "Location ID" minimum(1) maximum(2147483647) format(int32)
// @Param        request  body  location.UpdateLocationRequest   true  "Fields to merge-patch"
// @Success      200  {object}  locations.UpdateLocationResponse
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      415  {object}  modelerrors.ErrorResponse     "unsupported_media_type"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     BearerAuth[locations:write]
// @Router       /api/v1/locations/{location_id} [patch]
func (handler *Handler) Update(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doUpdate(w, req, orgID, id)
}

func (handler *Handler) doUpdate(w http.ResponseWriter, req *http.Request, orgID, id int) {
	reqID := middleware.GetRequestID(req.Context())

	// TRA-686 / BB29 F7: reject `tags` pre-decode with 400 invalid_value —
	// tags are managed via the /locations/{id}/tags subresource.
	//
	// TRA-699 (BB31 §2): `external_key` and `parent_external_key` are no
	// longer on this reject list. Both follow the uniform
	// accept-if-matches, reject-if-differs rule implemented below.
	if httputil.RejectFields(w, req, reqID, location.PublicRejectPatchFields) {
		return
	}

	var request location.UpdateLocationRequest
	// TRA-692: capture presentKeys alongside explicitNulls so the validator
	// response can promote any future length-bearing required violation to
	// code=required for omitted/null cases. Consistent with POST.
	explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, location.PublicReadOnlyFields)
	if err != nil {
		httputil.RespondDecodeError(w, req, err, reqID)
		return
	}

	// valid_from / name are non-nullable on the read view; an explicit null
	// in the PATCH body is a validation error, not a clear-request.
	//
	// TRA-702 / BB32 D3: accumulate every null-on-non-nullable violation
	// before responding so the integrator sees the full picture in one round
	// trip instead of replaying the request as they peel off one violation
	// per response.
	var nullViolations []modelerrors.FieldError
	for _, f := range []string{"valid_from", "name", "is_active"} {
		if _, ok := explicitNulls[f]; ok {
			nullViolations = append(nullViolations, modelerrors.FieldError{
				Field:   f,
				Code:    "invalid_value",
				Message: fmt.Sprintf("%s cannot be null; omit the field to leave unchanged, or provide a value", f),
			})
		}
	}
	if len(nullViolations) > 0 {
		httputil.WriteValidationError(w, req, reqID, nullViolations)
		return
	}
	if _, ok := explicitNulls["valid_to"]; ok {
		request.ClearValidTo = true
	}
	if _, ok := explicitNulls["description"]; ok {
		request.ClearDescription = true
	}
	// TRA-614 / BB19 §S1: explicit `null` on parent_id clears the FK.
	// TRA-699 (BB31 §2): parent_external_key follows the new echo check
	// (see below); only parent_id signals a clear via the surrogate.
	if _, ok := explicitNulls["parent_id"]; ok {
		request.ClearParentID = true
		request.ParentID = nil
	}

	// TRA-699 (BB31 §2): natural-key echo check. Two fields are read-only
	// on PATCH but accept a verbatim echo of the current value as a silent
	// no-op so a GET → PATCH round-trip without an explicit strip succeeds.
	// A differing value is 400 read_only naming POST /locations/{id}/rename.
	//
	//   external_key         → mutates own natural key via /rename
	//   parent_external_key  → re-parent via /rename, or send parent_id
	current, err := handler.storage.GetLocationWithParentByID(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)
		return
	}
	if current == nil {
		httputil.Respond404(w, req, apierrors.LocationNotFound, reqID)
		return
	}
	var echoViolations []modelerrors.FieldError
	if _, present := presentKeys["external_key"]; present {
		matched := request.ExternalKey != nil && *request.ExternalKey == current.ExternalKey
		if !matched {
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "external_key",
				Code:    "read_only",
				Message: "external_key is immutable via PATCH; use POST /api/v1/locations/{location_id}/rename to change it",
			})
		}
	}
	if _, present := presentKeys["parent_external_key"]; present {
		_, bodyNull := explicitNulls["parent_external_key"]
		curNull := current.ParentExternalKey == nil
		matched := bodyNull && curNull
		if !bodyNull && !curNull && request.ParentExternalKey != nil && current.ParentExternalKey != nil &&
			*request.ParentExternalKey == *current.ParentExternalKey {
			matched = true
		}
		if !matched {
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "parent_external_key",
				Code:    "read_only",
				Message: "parent reference is immutable via PATCH; use POST /api/v1/locations/{location_id}/rename to re-parent, or send `parent_id` to change the parent via surrogate",
			})
		}
	}
	if len(echoViolations) > 0 {
		// TRA-702 / BB32 D2: detail must echo fields[0].Message — pre-TRA-702
		// the inline literal "validation failed" buried the redirect-to-/rename
		// message inside fields[0] where AI integrators were less likely to
		// read it. WriteValidationError owns the echo + suffix contract.
		httputil.WriteValidationError(w, req, reqID, echoViolations)
		return
	}
	request.ExternalKey = nil
	request.ParentExternalKey = nil
	delete(presentKeys, "external_key")
	delete(presentKeys, "parent_external_key")
	delete(explicitNulls, "external_key")
	delete(explicitNulls, "parent_external_key")

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationErrorWithPresence(w, req, err, reqID, presentKeys, explicitNulls)
		return
	}

	resolved, fErr := handler.resolveParent(req, orgID, request.ParentID, nil)
	if fErr != nil {
		if fErr.Code == "internal_error" {
			httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
				fErr.Message, reqID)

			return
		}
		// TRA-681: fk_not_found is 400 validation_error (was 409 conflict
		// under TRA-678). See assets.Create for rationale.
		httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{*fErr})

		return
	}
	request.ParentID = resolved

	result, err := handler.storage.UpdateLocation(req.Context(), orgID, id, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
				err.Error(), reqID)

			return
		}
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}

	if result == nil {
		httputil.Respond404(w, req, apierrors.LocationNotFound, reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": location.ToPublicLocationView(*result)})
}

// @Summary Delete location
// @Description Delete a location by its ID. Returns 204 on success, 404 if the location does not exist or has already been deleted, and 409 if the location has descendant locations or assets placed directly at it. Descendants must be reassigned or removed and placed assets must be moved or removed before their parent location can be deleted; bulk cascade is not supported.
// @Tags locations,public
// @ID locations.delete
// @Accept json
// @Produce json
// @Param location_id path int true "Location ID" minimum(1) maximum(2147483647) format(int32)
// @Success 204 "deleted"
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 409 {object} modelerrors.ErrorResponse "conflict — has descendants or placed assets"
// @Failure 429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security BearerAuth[locations:write]
// @Router /api/v1/locations/{location_id} [delete]
func (handler *Handler) Delete(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doDelete(w, req, orgID, id)
}

func (handler *Handler) doDelete(w http.ResponseWriter, req *http.Request, orgID, id int) {
	reqID := middleware.GetRequestID(req.Context())

	// Pre-check: refuse to delete a location that would orphan descendants
	// or leave placed assets pointing at a soft-deleted location (TRA-644 /
	// BB22 F2). Distinct detail strings let integrators react correctly —
	// reassign descendants vs move assets are different remediations. v1
	// has no ?cascade=true; bulk is a separate ticket if customers ask.
	childCount, err := handler.storage.CountActiveChildLocations(req.Context(), orgID, id)
	if err != nil {
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}
	if childCount > 0 {
		httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
			"location has descendant locations; reassign or remove them before deleting (cascade is not supported)",
			reqID)
		return
	}

	assetCount, err := handler.storage.CountActiveAssetsAtLocation(req.Context(), orgID, id)
	if err != nil {
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}
	if assetCount > 0 {
		httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
			"location has assets placed at it; move or remove them before deleting (cascade is not supported)",
			reqID)
		return
	}

	deleted, err := handler.storage.DeleteLocation(req.Context(), orgID, id)
	if err != nil {
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}

	if !deleted {
		httputil.Respond404(w, req, apierrors.LocationNotFound, reqID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type ListLocationsResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}

type GetLocationResponse struct {
	Data location.PublicLocationView `json:"data"`
}

type CreateLocationResponse struct {
	Data location.PublicLocationView `json:"data"`
}

type UpdateLocationResponse struct {
	Data location.PublicLocationView `json:"data"`
}

// RenameLocationResponse is the typed envelope returned by
// POST /api/v1/locations/{location_id}/rename. `descendant_count_affected`
// reports the number of live descendant rows reachable through
// parent_location_id so integrators can decide whether to re-fetch the
// subtree (their downstream natural-key joins may need refreshing even
// though no descendant row was modified on the server). Does not include
// the renamed row itself; same-value rename returns 0. TRA-664 / TRA-684.
type RenameLocationResponse struct {
	Data                    location.PublicLocationView `json:"data"`
	DescendantCountAffected int                         `json:"descendant_count_affected" example:"7"`
}

// @Summary      Rename a location (mutate external_key)
// @Description  **Required scope:** `locations:write`
// @Description
// @Description  Mutate the location's `external_key`. This operation is **destructive to downstream joins** because consumers of the natural key must re-resolve it. Only this row's `external_key` changes on the server; descendants are not modified.
// @Description
// @Description  The response includes `descendant_count_affected` (the live descendant count reachable through `parent_id`) so an integrator can decide whether to refresh derived natural-key joins for the subtree. `external_key` is immutable via PATCH; this operation is the only way to change it. Distinct from a regular PATCH in audit logs (different URL surface).
// @Tags         locations,public
// @ID           locations.rename
// @Accept       json
// @Produce      json
// @Param        location_id path  int                              true  "Location ID" minimum(1) maximum(2147483647) format(int32)
// @Param        request     body  location.RenameLocationRequest   true  "New external_key"
// @Success      200  {object}  locations.RenameLocationResponse
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      415  {object}  modelerrors.ErrorResponse     "unsupported_media_type"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     BearerAuth[locations:write]
// @Router       /api/v1/locations/{location_id}/rename [post]
func (handler *Handler) Rename(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	var request location.RenameLocationRequest
	// TRA-692: presence-tracking decoder so an omitted or null `external_key`
	// surfaces as code=required (not the TRA-675-collapsed too_short). Body
	// has a single required string field, exactly the §1.2 case.
	explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, nil)
	if err != nil {
		httputil.RespondDecodeError(w, req, err, reqID)
		return
	}
	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationErrorWithPresence(w, req, err, reqID, presentKeys, explicitNulls)
		return
	}

	result, descendantCount, err := handler.storage.RenameLocation(req.Context(), orgID, id, request.ExternalKey)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
				err.Error(), reqID)
			return
		}
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}
	if result == nil {
		httputil.Respond404(w, req, apierrors.LocationNotFound, reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":                      location.ToPublicLocationView(*result),
		"descendant_count_affected": descendantCount,
	})
}

type ListAncestorsResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}

type ListChildrenResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}

type ListDescendantsResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}

// @Summary List locations
// @Description Paginated locations list with natural-key filters, sort, and substring search.
// @Description
// @Description Default scope returns currently-effective locations only — rows whose `valid_from` is in the past AND whose `valid_to` is null or in the future. The `is_active` filter is independent of temporal validity; omit it to include both active and inactive rows within the effective window, or pass `?is_active=true`/`false` to filter further.
// @Tags locations,public
// @ID locations.list
// @Param limit               query int    false "max 200"  default(50) minimum(1) maximum(200)
// @Param offset              query int    false "min 0"   default(0) minimum(0)
// @Param parent_id            query []int    false "filter by parent id (canonical, may repeat); mutually exclusive with parent_external_key (400 ambiguous_fields if both supplied)" collectionFormat(multi)
// @Param parent_external_key query []string false "filter by parent's external_key (may repeat); mutually exclusive with parent_id (400 ambiguous_fields if both supplied)" collectionFormat(multi)
// @Param external_key         query []string false "filter by location external_key, equality match (may repeat for any-of)" collectionFormat(multi)
// @Param is_active           query bool   false "filter by active flag"
// @Param include_deleted     query bool   false "when true, include soft-deleted rows in the response. deleted_at is populated for those rows. Orthogonal to is_active." default(false)
// @Param q                   query string false "substring search (case-insensitive) on name, external_key, description, and active tag values"
// @Param sort                query []string false "comma-separated, prefix '-' for DESC" collectionFormat(csv) Enums(external_key, -external_key, name, -name, created_at, -created_at)
// @Success 200 {object} locations.ListLocationsResponse
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security BearerAuth[locations:read]
// @Router /api/v1/locations [get]
func (handler *Handler) ListLocations(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Filters:     []string{"parent_id", "parent_external_key", "external_key", "is_active", "include_deleted", "q"},
		BoolFilters: []string{"is_active", "include_deleted"},
		Sorts:       []string{"external_key", "name", "created_at"},
	})
	if err != nil {
		httputil.RespondListParamError(w, req, err, reqID)
		return
	}

	// TRA-681: parent_id and parent_external_key form a oneOf on the GET
	// filter — reject 400 ambiguous_fields when both are supplied so
	// integrators get a typed signal rather than a silent winner. OpenAPI 3
	// can't encode a query-parameter pair constraint, so the rule lives in
	// the per-parameter description and the handler validation here.
	_, hasParentID := params.Filters["parent_id"]
	_, hasParentExt := params.Filters["parent_external_key"]
	if hasParentID && hasParentExt {
		httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{
			{Field: "parent_id", Code: "ambiguous_fields", Message: "parent_id and parent_external_key are mutually exclusive; supply exactly one"},
			{Field: "parent_external_key", Code: "ambiguous_fields", Message: "parent_id and parent_external_key are mutually exclusive; supply exactly one"},
		})
		return
	}

	f := location.ListFilter{
		ParentExternalKeys: params.Filters["parent_external_key"],
		ExternalKeys:       params.Filters["external_key"],
		Limit:              params.Limit,
		Offset:             params.Offset,
	}
	if vs, ok := params.Filters["parent_id"]; ok && len(vs) > 0 {
		f.ParentIDs = make([]int, 0, len(vs))
		for _, s := range vs {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || int64(n) > httputil.SurrogateIDMax {
				httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{{
					Field:   "parent_id",
					Code:    "invalid_value",
					Message: fmt.Sprintf("parent_id %q must be a positive int32", s),
				}})
				return
			}
			f.ParentIDs = append(f.ParentIDs, n)
		}
	}
	if vs, ok := params.Filters["is_active"]; ok && len(vs) > 0 {
		b := vs[0] == "true"
		f.IsActive = &b
	}
	if vs, ok := params.Filters["include_deleted"]; ok && len(vs) > 0 {
		f.IncludeDeleted = vs[0] == "true"
	}
	if vs, ok := params.Filters["q"]; ok && len(vs) > 0 {
		f.Q = &vs[0]
	}
	for _, s := range params.Sorts {
		f.Sorts = append(f.Sorts, location.ListSort{Field: s.Field, Desc: s.Desc})
	}

	items, err := handler.storage.ListLocationsFiltered(req.Context(), orgID, f)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	total, err := handler.storage.CountLocationsFiltered(req.Context(), orgID, f)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	out := make([]location.PublicLocationView, 0, len(items))
	for _, l := range items {
		out = append(out, location.ToPublicLocationView(l))
	}

	httputil.WriteJSON(w, http.StatusOK, ListLocationsResponse{
		Data:       out,
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}

// @Summary Get location by ID
// @Description Retrieve a location by its canonical ID. Returns 404 if not found.
// @Description
// @Description Path-addressed retrieval bypasses the temporal-validity filter applied on list endpoints — any non-deleted location is returned regardless of its `valid_from` / `valid_to` values. Use this endpoint when you have an id and need the row even if its effective window has elapsed.
// @Tags locations,public
// @ID locations.get
// @Param location_id path int true "Location ID" minimum(1) maximum(2147483647) format(int32)
// @Success 200 {object} locations.GetLocationResponse
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 429 {object} modelerrors.ErrorResponse
// @Security BearerAuth[locations:read]
// @Router /api/v1/locations/{location_id} [get]
func (handler *Handler) GetLocation(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, err := httputil.ParseSurrogateID("location_id", chi.URLParam(req, "location_id"))
	if err != nil {
		httputil.RespondPathParamError(w, req, err, reqID)
		return
	}

	view, err := handler.storage.GetLocationViewByID(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}
	if view == nil {
		httputil.Respond404(w, req, apierrors.LocationNotFound, reqID)
		return
	}

	withParent := location.LocationWithParent{LocationView: *view}
	if view.ParentID != nil {
		parent, err := handler.storage.GetLocationByID(req.Context(), orgID, *view.ParentID)
		if err == nil && parent != nil {
			ek := parent.ExternalKey
			withParent.ParentExternalKey = &ek
		}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": location.ToPublicLocationView(withParent)})
}

// @Summary List location ancestors
// @Description Sort order is fixed: ancestors are returned root first (walking up the `parent_id` chain), with `id` ascending as a deterministic tiebreaker. No `sort` query parameter is exposed because the natural order toward the root is the only meaningful order for this list.
// @Tags locations,public
// @ID locations.ancestors
// @Param location_id path  int    true  "Location ID" minimum(1) maximum(2147483647) format(int32)
// @Param limit  query int    false "max 200"  default(50) minimum(1) maximum(200)
// @Param offset query int    false "min 0"   default(0) minimum(0)
// @Success 200 {object} locations.ListAncestorsResponse
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security BearerAuth[locations:read]
// @Router /api/v1/locations/{location_id}/ancestors [get]
func (handler *Handler) GetAncestors(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	if err != nil {
		httputil.RespondListParamError(w, req, err, reqID)
		return
	}

	results, err := handler.storage.ListAncestorsPaginated(req.Context(), orgID, id, params.Limit, params.Offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	total, err := handler.storage.CountAncestors(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	httputil.WriteJSON(w, http.StatusOK, ListAncestorsResponse{
		Data:       toPublicLocationViews(results),
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}

// @Summary List location descendants
// @Description Sort order is fixed: descendants are returned in depth-first tree order (each level sorted by lowercased `external_key`), with `id` ascending as a deterministic tiebreaker. No `sort` query parameter is exposed because the depth-first tree walk is the only meaningful order for this list.
// @Tags locations,public
// @ID locations.descendants
// @Param location_id path  int    true  "Location ID" minimum(1) maximum(2147483647) format(int32)
// @Param limit  query int    false "max 200"  default(50) minimum(1) maximum(200)
// @Param offset query int    false "min 0"   default(0) minimum(0)
// @Success 200 {object} locations.ListDescendantsResponse
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security BearerAuth[locations:read]
// @Router /api/v1/locations/{location_id}/descendants [get]
func (handler *Handler) GetDescendants(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	if err != nil {
		httputil.RespondListParamError(w, req, err, reqID)
		return
	}

	results, err := handler.storage.ListDescendantsPaginated(req.Context(), orgID, id, params.Limit, params.Offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	total, err := handler.storage.CountDescendants(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	httputil.WriteJSON(w, http.StatusOK, ListDescendantsResponse{
		Data:       toPublicLocationViews(results),
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}

// @Summary List location children
// @Description Sort order is fixed: immediate children are returned ordered alphabetically by `name` ascending, with `id` ascending as a deterministic tiebreaker when sibling names collide. No `sort` query parameter is exposed because alphabetical-by-name is the only meaningful order for a single level of siblings.
// @Tags locations,public
// @ID locations.children
// @Param location_id path  int    true  "Location ID" minimum(1) maximum(2147483647) format(int32)
// @Param limit  query int    false "max 200"  default(50) minimum(1) maximum(200)
// @Param offset query int    false "min 0"   default(0) minimum(0)
// @Success 200 {object} locations.ListChildrenResponse
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security BearerAuth[locations:read]
// @Router /api/v1/locations/{location_id}/children [get]
func (handler *Handler) GetChildren(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	if err != nil {
		httputil.RespondListParamError(w, req, err, reqID)
		return
	}

	results, err := handler.storage.ListChildrenPaginated(req.Context(), orgID, id, params.Limit, params.Offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	total, err := handler.storage.CountChildren(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	httputil.WriteJSON(w, http.StatusOK, ListChildrenResponse{
		Data:       toPublicLocationViews(results),
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}

func toPublicLocationViews(locs []location.LocationWithParent) []location.PublicLocationView {
	views := make([]location.PublicLocationView, len(locs))
	for i, l := range locs {
		views[i] = location.ToPublicLocationView(l)
	}
	return views
}

type AddTagResponse struct {
	Data shared.Tag `json:"data"`
}

// @Summary Add a tag to a location
// @Tags locations,public
// @ID locations.tags.add
// @Accept json
// @Param location_id path int               true "Location ID" minimum(1) maximum(2147483647) format(int32)
// @Param request body shared.TagRequest true "Tag to attach"
// @Success 201 {object} locations.AddTagResponse "tag attached"
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 409 {object} modelerrors.ErrorResponse "conflict"
// @Failure 415 {object} modelerrors.ErrorResponse "unsupported_media_type"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security BearerAuth[locations:write]
// @Router /api/v1/locations/{location_id}/tags [post]
func (handler *Handler) AddTag(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, r, orgID, requestID)
	if !ok {
		return
	}

	handler.doAddLocationTag(w, r, orgID, id)
}

func (handler *Handler) doAddLocationTag(w http.ResponseWriter, r *http.Request, orgID, locationID int) {
	requestID := middleware.GetRequestID(r.Context())

	var request shared.TagRequest
	// TRA-692: presence-tracking decoder so an omitted `value` surfaces as
	// code=required.
	explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(r, &request, nil)
	if err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationErrorWithPresence(w, r, err, requestID, presentKeys, explicitNulls)
		return
	}

	tag, err := handler.storage.AddTagToLocation(r.Context(), orgID, locationID, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				err.Error(), requestID)

			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, AddTagResponse{Data: *tag})
}

// @Summary Remove a tag from a location
// @Description Detach a tag from a location by its tag record id.
// @Description Idempotent: returns 204 whether or not the tag was associated. Repeated calls are safe.
// @Tags locations,public
// @ID locations.tags.remove
// @Param location_id path int true "Location ID" minimum(1) maximum(2147483647) format(int32)
// @Param tag_id      path int true "Tag ID" minimum(1) maximum(2147483647) format(int32)
// @Success 204 "deleted"
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security BearerAuth[locations:write]
// @Router /api/v1/locations/{location_id}/tags/{tag_id} [delete]
func (handler *Handler) RemoveTag(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	id, err := httputil.ParseSurrogateID("location_id", chi.URLParam(r, "location_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, requestID)
		return
	}

	loc, err := handler.storage.GetLocationByID(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), requestID)

		return
	}
	if loc == nil || loc.OrgID != orgID {
		httputil.Respond404(w, r, apierrors.LocationNotFound, requestID)
		return
	}

	handler.doRemoveLocationTag(w, r, orgID, loc.ID)
}

func (handler *Handler) doRemoveLocationTag(w http.ResponseWriter, r *http.Request, orgID, locationID int) {
	requestID := middleware.GetRequestID(r.Context())

	tagID, err := httputil.ParseSurrogateID("tag_id", chi.URLParam(r, "tag_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, requestID)
		return
	}

	_, err = handler.storage.RemoveLocationTag(r.Context(), orgID, locationID, tagID)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseAndVerifyLocationID extracts {location_id}, parses it as a surrogate
// int, and verifies the location exists and belongs to the caller's org.
func (handler *Handler) parseAndVerifyLocationID(w http.ResponseWriter, req *http.Request, orgID int, reqID string) (int, bool) {
	id, err := httputil.ParseSurrogateID("location_id", chi.URLParam(req, "location_id"))
	if err != nil {
		httputil.RespondPathParamError(w, req, err, reqID)
		return 0, false
	}

	loc, err := handler.storage.GetLocationByID(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return 0, false
	}
	if loc == nil || loc.OrgID != orgID {
		httputil.Respond404(w, req, apierrors.LocationNotFound, reqID)
		return 0, false
	}

	return loc.ID, true
}
