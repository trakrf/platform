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
// parent_external_key (natural-key alternate) inputs on Create and PATCH.
// Exactly one is expected to be set on Create (TRA-681 oneOf constraint
// enforced upstream via the ambiguous_fields pre-check). TRA-719 / BB35
// B2: PATCH now also dispatches the natural-key form here — the PATCH
// handler runs its own ambiguous_fields pre-check before this call.
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
// @Description  Both forms may be supplied together when they name the same parent (silently normalized to a single re-parent operation); a disagreement returns 400 `ambiguous_fields`.
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
// @Header       201  {string}  Location  "Path of the created resource (resolve against request URL per RFC 7231 §7.1.2)"
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

	// TRA-705 (BB32 §C6) + TRA-732 R2: every non-nullable field — including
	// the required string fields `name` and `external_key` — emits
	// `invalid_value` for explicit null. `required` is reserved for the
	// absent-key case; explicit null on a non-nullable field is "you sent
	// a bad value," not "you forgot to include this." Accumulate every
	// violation in one response (BB32 §D3 pattern).
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
	for _, f := range []string{"name", "external_key"} {
		if _, ok := explicitNulls[f]; ok {
			nullViolations = append(nullViolations, modelerrors.FieldError{
				Field:   f,
				Code:    "invalid_value",
				Message: fmt.Sprintf("%s cannot be null; provide a string value", f),
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

	// TRA-757 (BB50/51/52 F1): reconcile parent_id and parent_external_key
	// when both supplied on Create. Symmetric with the PATCH reconciliation
	// below — a payload that names the same parent through both forms is
	// silently normalized to a single re-parent operation, and only a
	// disagreement returns 400 ambiguous_fields. The "POST-the-GET-response
	// after editing one field" workflow is encouraged elsewhere in the docs
	// and is a natural integrator pattern; pre-TRA-757 POST was the one place
	// it broke. Mixed null/non-null intent stays ambiguous on Create too.
	_, hasParentID := presentKeys["parent_id"]
	_, hasParentExt := presentKeys["parent_external_key"]
	if hasParentID && hasParentExt {
		_, idNull := explicitNulls["parent_id"]
		_, extNull := explicitNulls["parent_external_key"]
		switch {
		case idNull && extNull:
			// Both null — equivalent to "no parent." Drop the natural-key
			// form so resolveParent below sees a single intent.
			request.ParentExternalKey = nil
		case !idNull && !extNull:
			// Both non-null: resolve the natural key and verify it matches
			// the supplied surrogate. resolveParent returns fk_not_found if
			// the natural key doesn't exist.
			extResolved, fErr := handler.resolveParent(r, orgID, nil, request.ParentExternalKey)
			if fErr != nil {
				if fErr.Code == "internal_error" {
					httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, fErr.Message, requestID)
					return
				}
				httputil.WriteValidationError(w, r, requestID, []modelerrors.FieldError{*fErr})
				return
			}
			if extResolved == nil || request.ParentID == nil || *extResolved != *request.ParentID {
				httputil.WriteValidationError(w, r, requestID, []modelerrors.FieldError{
					{Field: "parent_id", Code: "ambiguous_fields", Message: "parent_id and parent_external_key both supplied and disagree; supply exactly one or supply consistent values"},
					{Field: "parent_external_key", Code: "ambiguous_fields", Message: "parent_id and parent_external_key both supplied and disagree; supply exactly one or supply consistent values"},
				})
				return
			}
			// Consistent — drop the natural-key form and proceed with the
			// surrogate.
			request.ParentExternalKey = nil
		default:
			// Mixed intent (one null, one non-null): ambiguous.
			httputil.WriteValidationError(w, r, requestID, []modelerrors.FieldError{
				{Field: "parent_id", Code: "ambiguous_fields", Message: "parent_id and parent_external_key are mutually exclusive when one is null and the other is set; supply exactly one"},
				{Field: "parent_external_key", Code: "ambiguous_fields", Message: "parent_id and parent_external_key are mutually exclusive when one is null and the other is set; supply exactly one"},
			})
			return
		}
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

	// TRA-765 (BB56 F3): reject inverted or instantaneous validity windows.
	// valid_from has been defaulted above so the comparison runs against an
	// effective non-zero value.
	var locValidTo *time.Time
	if request.ValidTo != nil {
		t := request.ValidTo.ToTime()
		locValidTo = &t
	}
	if fe := httputil.ValidateValidityWindow(request.ValidFrom.ToTime(), locValidTo); fe != nil {
		httputil.WriteValidationError(w, r, requestID, []modelerrors.FieldError{*fe})
		return
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
// @Description  Apply a JSON Merge Patch (RFC 7396) to a location. Only fields included in the request body are changed; fields set to `null` clear the corresponding nullable column. Omitted fields are left unchanged. Every accepted PATCH — empty body (`{}`), verbatim echo of current values, partial mutation, or full mutation — advances `updated_at` on success (filesystem `touch` semantics). Read-only fields are uniformly governed by the accept-if-matches, reject-if-differs rule: a value matching the current resource state is silently normalized out (so a verbatim GET → PATCH round-trip succeeds without manual scrubbing), and a differing value returns 400. The rejection `code` splits the two semantic classes: server-managed fields (`id`, `created_at`, `updated_at`, `deleted_at`) return `code: read_only` — they have no public mutation path. Fields mutable via a sub-resource verb (`external_key`, `tags`) return `code: invalid_context` and the detail names the correct verb: mutate `external_key` via POST /locations/{location_id}/rename; mutate `tags` via POST /locations/{location_id}/tags and DELETE /locations/{location_id}/tags/{tag_id}. The `tags` collection is compared as a set on full tag content — array ordering is not significant; differing set membership or differing field values on a matching id returns 400 `invalid_context`. To re-parent, send `parent_id` (surrogate) OR `parent_external_key` (natural key), or both forms in the same body provided they resolve to the same parent (silently normalized to a single re-parent operation, symmetric with CreateLocationRequest); either form accepts `null` to clear the FK, and disagreement between the two forms returns 400 `ambiguous_fields`.
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

	// TRA-710 (BB33 F2): server-managed read-only fields (id, created_at,
	// updated_at, deleted_at, tags) follow the accept-if-matches /
	// reject-if-differs rule. Peek raw body values before strict decode
	// strips id/created_at/updated_at/deleted_at and before the
	// UpdateLocationRequest decode silently ignores tags. Compared against
	// current resource state below.
	rawReadOnly := httputil.PeekJSONFields(req, []string{
		"id", "created_at", "updated_at", "deleted_at", "tags",
	})

	// PublicRejectPatchFields is currently empty (tags moved to the echo
	// check under TRA-710); kept as the call site for future fields whose
	// mere presence is invalid regardless of value.
	if httputil.RejectFields(w, req, reqID, location.PublicRejectPatchFields) {
		return
	}

	var request location.UpdateLocationRequest
	// TRA-692: capture presentKeys alongside explicitNulls so the validator
	// response can promote any future length-bearing required violation to
	// code=required for omitted/null cases. Consistent with POST.
	//
	// TRA-710: `tags` is added to the drop list alongside PublicReadOnlyFields
	// because UpdateLocationRequest has no `tags` field to decode into; the
	// echo check above already captured its raw value.
	dropFields := append([]string{"tags"}, location.PublicReadOnlyFields...)
	explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, dropFields)
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
	// TRA-719 / BB35 B2: parent_external_key is now writable on PATCH and
	// follows the same null-clears-FK semantic as parent_id. Both forms
	// signal a clear; the post-decode reconciliation below collapses them
	// into a single ClearParentID.
	if _, ok := explicitNulls["parent_id"]; ok {
		request.ClearParentID = true
		request.ParentID = nil
	}
	if _, ok := explicitNulls["parent_external_key"]; ok {
		request.ClearParentID = true
		request.ParentExternalKey = nil
	}

	// TRA-699 (BB31 §2): natural-key echo check. `external_key` is
	// read-only on PATCH but accepts a verbatim echo of the current value
	// as a silent no-op so a GET → PATCH round-trip without an explicit
	// strip succeeds. A differing value is 400 read_only naming POST
	// /locations/{location_id}/rename.
	//
	// TRA-719 / BB35 B2: `parent_external_key` is now writable on PATCH,
	// symmetric with CreateLocationRequest. Its value is reconciled with
	// `parent_id` below — they form a oneOf (ambiguous_fields if both
	// supplied), and the natural-key form resolves via resolveParent.
	//
	// TRA-710 (BB33 F2): the same accept-if-matches / reject-if-differs
	// rule covers the server-managed surrogate id and timestamps (id,
	// created_at, updated_at, deleted_at) and the `tags` collection. Their
	// raw body values were peeked above (rawReadOnly) so the comparison
	// here runs against the current resource state.
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
	// TRA-710: server-managed read-only echo checks. The current view is
	// projected through ToPublicLocationView so the wire shape we compare
	// against matches the wire shape an integrator GETs.
	currentView := location.ToPublicLocationView(*current)
	if v, present := rawReadOnly["id"]; present {
		if !httputil.SameJSON(v, currentView.ID) {
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "id",
				Code:    "read_only",
				Message: "id is server-assigned and immutable; submit the resource's current id or omit the field",
			})
		}
	}
	// TRA-721: read-only datetime echo checks use instant equality, not
	// byte equality — so a verbatim GET → typed-deserialize → PATCH
	// round-trip from a generated client (Go time.Time, Pydantic, etc.)
	// succeeds even when the re-serialized wire form differs from the
	// server's emit shape (e.g., "+00:00" vs "Z", microsecond fractional).
	if v, present := rawReadOnly["created_at"]; present {
		if !httputil.SameJSONInstant(v, currentView.CreatedAt) {
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "created_at",
				Code:    "read_only",
				Message: "created_at is server-managed and immutable; submit the resource's current created_at or omit the field",
			})
		}
	}
	if v, present := rawReadOnly["updated_at"]; present {
		if !httputil.SameJSONInstant(v, currentView.UpdatedAt) {
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "updated_at",
				Code:    "read_only",
				Message: "updated_at is server-managed; PATCH advances it implicitly. Submit the resource's current updated_at or omit the field",
			})
		}
	}
	if v, present := rawReadOnly["deleted_at"]; present {
		if !httputil.SameJSONInstant(v, currentView.DeletedAt) {
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "deleted_at",
				Code:    "read_only",
				Message: "deleted_at is server-managed; use DELETE /api/v1/locations/{location_id} to soft-delete. Submit the resource's current deleted_at or omit the field",
			})
		}
	}
	if v, present := rawReadOnly["tags"]; present {
		// TRA-775 (BB61-3 F1): tags PATCH echo is compared as a set, not a
		// sequence. A submitted array with the same tag content as the
		// current state matches regardless of element order, so generated
		// clients that deserialize tags into unordered collections (Python
		// set, Go map, ORMs with hash-ordered associations) succeed on a
		// verbatim GET → PATCH round-trip without manual sort. Differing
		// set membership or differing field values on a matching id still
		// returns 400 read_only.
		if !httputil.SameTagSet(v, currentView.Tags) {
			// TRA-780 F4: `tags` is mutable on the location surface — just
			// via POST/DELETE on the /tags sub-resource, not via PATCH.
			// Emit `invalid_context` rather than `read_only`. Detail
			// unchanged.
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "tags",
				Code:    "invalid_context",
				Message: "the tags field on PATCH must equal the current value as a set (idempotent echo only; ordering is not significant); use POST /api/v1/locations/{location_id}/tags and DELETE /api/v1/locations/{location_id}/tags/{tag_id} to mutate",
			})
		}
	}
	if _, present := presentKeys["external_key"]; present {
		matched := request.ExternalKey != nil && *request.ExternalKey == current.ExternalKey
		if !matched {
			// TRA-780 F4: external_key is mutable via POST /rename — emit
			// `invalid_context` rather than `read_only`. Detail unchanged.
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "external_key",
				Code:    "invalid_context",
				Message: `external_key is immutable via PATCH; use POST /api/v1/locations/{location_id}/rename with body {"external_key": "<new value>"} to change it`,
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
	delete(presentKeys, "external_key")
	delete(explicitNulls, "external_key")

	// TRA-719 / BB35 B2: reconcile parent_id and parent_external_key when
	// both supplied. The BB33 F2 accept-if-matches rule still applies —
	// a verbatim GET → PATCH round-trip carries both fields and must
	// succeed when they describe the same parent. Disagreement (or mixed
	// "one clear, one set" intent) is ambiguous_fields.
	_, hasParentID := presentKeys["parent_id"]
	_, hasParentExt := presentKeys["parent_external_key"]
	if hasParentID && hasParentExt {
		_, idNull := explicitNulls["parent_id"]
		_, extNull := explicitNulls["parent_external_key"]
		switch {
		case idNull && extNull:
			// Both clear — ClearParentID is already set above via the
			// per-field null branches. Drop the natural-key form so
			// resolveParent below sees a single intent.
			request.ParentExternalKey = nil
		case !idNull && !extNull:
			// Both non-null: resolve the natural key and check it
			// matches the supplied surrogate. resolveParent returns
			// fk_not_found if the natural key doesn't exist.
			extResolved, fErr := handler.resolveParent(req, orgID, nil, request.ParentExternalKey)
			if fErr != nil {
				if fErr.Code == "internal_error" {
					httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal, fErr.Message, reqID)
					return
				}
				httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{*fErr})
				return
			}
			if extResolved == nil || request.ParentID == nil || *extResolved != *request.ParentID {
				httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{
					{Field: "parent_id", Code: "ambiguous_fields", Message: "parent_id and parent_external_key both supplied and disagree; supply exactly one or supply consistent values"},
					{Field: "parent_external_key", Code: "ambiguous_fields", Message: "parent_id and parent_external_key both supplied and disagree; supply exactly one or supply consistent values"},
				})
				return
			}
			// Consistent — drop the natural-key form and proceed with
			// the surrogate. ClearParentID remains false.
			request.ParentExternalKey = nil
		default:
			// Mixed intent (one null, one non-null): ambiguous.
			httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{
				{Field: "parent_id", Code: "ambiguous_fields", Message: "parent_id and parent_external_key are mutually exclusive when one is null and the other is set; supply exactly one"},
				{Field: "parent_external_key", Code: "ambiguous_fields", Message: "parent_id and parent_external_key are mutually exclusive when one is null and the other is set; supply exactly one"},
			})
			return
		}
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationErrorWithPresence(w, req, err, reqID, presentKeys, explicitNulls)
		return
	}

	// TRA-765 (BB56 F3): reject inverted or instantaneous validity windows on
	// PATCH. Effective valid_from is the body value when supplied else the
	// current value; effective valid_to is nil when the body clears it, the
	// body value when present and non-null, else the current value.
	effectiveValidFrom := current.ValidFrom
	if request.ValidFrom != nil {
		effectiveValidFrom = request.ValidFrom.ToTime()
	}
	var effectiveValidTo *time.Time
	switch {
	case request.ClearValidTo:
		effectiveValidTo = nil
	case request.ValidTo != nil:
		t := request.ValidTo.ToTime()
		effectiveValidTo = &t
	default:
		effectiveValidTo = current.ValidTo
	}
	if fe := httputil.ValidateValidityWindow(effectiveValidFrom, effectiveValidTo); fe != nil {
		httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{*fe})
		return
	}

	// TRA-719 / BB35 B2: dispatch parent_external_key through the same FK
	// resolution used at create time. resolveParent returns the resolved
	// surrogate id (or a fk_not_found field error if the natural key has
	// no matching live row).
	resolved, fErr := handler.resolveParent(req, orgID, request.ParentID, request.ParentExternalKey)
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

	// TRA-770 BB58 F1: when the PATCH actually changes parent_id to a non-null
	// value, reject any assignment that would create a cycle. Routes both the
	// 1-hop self-parent case and the N-hop transitive case through the same
	// 409 path with a specific actionable detail (TRA-770 BB58 F2 folded in).
	// A null parent_id (root-promotion) and a no-op same-parent assignment
	// can't form cycles and skip the check.
	if resolved != nil {
		sameParent := current.ParentID != nil && *current.ParentID == *resolved
		if !sameParent {
			wouldCycle, cycErr := handler.storage.WouldCreateLocationCycle(req.Context(), orgID, id, *resolved)
			if cycErr != nil {
				httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
					cycErr.Error(), reqID)
				return
			}
			if wouldCycle {
				var detail string
				if *resolved == id {
					detail = fmt.Sprintf("parent_id %d would create a self-referential cycle", id)
				} else {
					detail = fmt.Sprintf("parent_id %d would create a cycle through location %d", *resolved, id)
				}
				httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict, detail, reqID)
				return
			}
		}
	}

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

	// TRA-713 / BB33 F5+C2: external_key-style filters must enforce the
	// same regex the field validators apply on POST/PATCH. Without this,
	// a slash-containing (or otherwise non-conforming) value silently
	// returns 200-with-empty rather than 400 invalid_value, masking
	// integration bugs at the boundary.
	if fe := httputil.ValidateExternalKeyFilterValues("external_key", params.Filters["external_key"]); fe != nil {
		httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{*fe})
		return
	}
	if fe := httputil.ValidateExternalKeyFilterValues("parent_external_key", params.Filters["parent_external_key"]); fe != nil {
		httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{*fe})
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
			if err != nil || n < 1 {
				httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{{
					Field:   "parent_id",
					Code:    "invalid_value",
					Message: fmt.Sprintf("parent_id %q must be a positive integer", s),
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
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
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

	if cycErr := handler.guardTreeCycle(w, req, orgID, id, reqID); cycErr {
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

	if cycErr := handler.guardTreeCycle(w, req, orgID, id, reqID); cycErr {
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

// guardTreeCycle pre-checks the location tree from `id` (both up and down)
// for a cycle in `parent_location_id`. With the TRA-770 BB58 F1 write-time
// check in place the tree should always be acyclic; this is the read-time
// defense in depth so a corrupt-tree state surfaces as a 500 with a
// diagnostic detail instead of hanging the recursive walker forever.
// Returns true when the response has been written and the caller must stop.
func (handler *Handler) guardTreeCycle(w http.ResponseWriter, req *http.Request, orgID, id int, reqID string) bool {
	hasCycle, err := handler.storage.DetectLocationTreeCycle(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)
		return true
	}
	if hasCycle {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			fmt.Sprintf("location %d is part of a parent_id cycle; tree walk aborted", id), reqID)
		return true
	}
	return false
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
// @Header  201 {string} Location "Path of the created tag (resolve against request URL per RFC 7231 §7.1.2)"
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

	// TRA-707 / BB32 C2: emit Location pointing at the newly created tag
	// subresource (RFC 7231 §7.1.2). Matches the canonical-URL pattern on
	// POST /api/v1/locations.
	w.Header().Set("Location", fmt.Sprintf("/api/v1/locations/%d/tags/%d", locationID, tag.ID))
	httputil.WriteJSON(w, http.StatusCreated, AddTagResponse{Data: *tag})
}

// @Summary Remove a tag from a location
// @Description Detach a tag from a location by its tag record id.
// @Description First successful removal returns 204; repeated calls return 404 — consistent with top-level resource DELETE semantics. The cross-location / cross-org case (a tag that exists but is not attached to this location, or belongs to a different org) also surfaces as 404.
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

	removed, err := handler.storage.RemoveLocationTag(r.Context(), orgID, locationID, tagID)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}
	// TRA-719 / BB35 A3: align tag subresource DELETE with top-level
	// DELETE semantics — second call returns 404, not 204. The cross-
	// location and cross-org cases also fall here (storage guard returns
	// removed=false rather than an error).
	if !removed {
		httputil.Respond404(w, r, "Tag not found on this location", requestID)
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
