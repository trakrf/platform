package assets

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
	"github.com/trakrf/platform/backend/internal/models/asset"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/services/bulkimport"
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
	storage           *storage.Storage
	bulkImportService *bulkimport.Service
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{
		storage:           storage,
		bulkImportService: bulkimport.NewService(storage),
	}
}

// resolveLocation reconciles the location_id (canonical surrogate) and
// location_external_key (natural-key alternate) inputs on Create. Exactly
// one is expected to be set (TRA-681 oneOf constraint enforced upstream
// in the Create handler via the ambiguous_fields pre-check). PATCH never
// supplies the natural-key form — it is stripped before this function
// runs (see asset.PublicReadOnlyFields).
//
// Both nil → nil (no location). When location_id is set it is used;
// otherwise location_external_key is resolved via lookup. Wire field
// names dropped the `current_` prefix in TRA-580 C-3.
//
// TRA-674 / BB27 F2 / TRA-681: a nonexistent surrogate `location_id`
// returns the same envelope shape as a nonexistent natural-key
// `location_external_key` — both surface keyed on the offending field
// as 400 validation_error with code=fk_not_found. Previously the
// surrogate path reached the storage layer and tripped the FK
// constraint, surfacing as 500 internal_error.
func (handler *Handler) resolveLocation(
	r *http.Request, orgID int, locID *int, locExternalKey *string,
) (*int, *modelerrors.FieldError) {
	hasID := locID != nil
	hasExt := locExternalKey != nil && *locExternalKey != ""

	if !hasID && !hasExt {
		return nil, nil
	}
	if hasID {
		loc, err := handler.storage.GetLocationByID(r.Context(), orgID, *locID)
		if err != nil {
			return nil, &modelerrors.FieldError{
				Field:   "location_id",
				Code:    "internal_error",
				Message: err.Error(),
			}
		}
		if loc == nil {
			return nil, &modelerrors.FieldError{
				Field:   "location_id",
				Code:    "fk_not_found",
				Message: fmt.Sprintf("location_id %d not found", *locID),
			}
		}
		return locID, nil
	}

	loc, err := handler.storage.GetLocationByExternalKey(r.Context(), orgID, *locExternalKey)
	if err != nil {
		return nil, &modelerrors.FieldError{
			Field:   "location_external_key",
			Code:    "internal_error",
			Message: err.Error(),
		}
	}
	if loc == nil {
		return nil, &modelerrors.FieldError{
			Field:   "location_external_key",
			Code:    "fk_not_found",
			Message: fmt.Sprintf("location_external_key %q not found", *locExternalKey),
		}
	}
	return &loc.ID, nil
}

// @Summary      Create an asset
// @Description  Create a new asset record, optionally with one or more tags (RFID, BLE, barcode).
// @Description
// @Description  The `external_key` field is optional. Provide a value from your system of record
// @Description  (ERP, WMS, asset management) for natural-key joins, or omit it to receive a
// @Description  server-assigned external_key in the format `ASSET-NNNN` (per-organization sequence).
// @Description  A caller-supplied external_key that collides with an existing asset returns 409.
// @Description
// @Description  Returns the created asset with its assigned tags. The Location response header contains the canonical URL.
// @Tags         assets,public
// @ID           assets.create
// @Accept       json
// @Produce      json
// @Param        request  body  asset.CreateAssetWithTagsRequest  true  "Asset to create with optional tags"
// @Success      201  {object}  assets.CreateAssetResponse
// @Header       201  {string}  Location  "Canonical URL of the created resource"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      415  {object}  modelerrors.ErrorResponse     "unsupported_media_type"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     BearerAuth[assets:write]
// @Router       /api/v1/assets [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	var request asset.CreateAssetWithTagsRequest
	// TRA-692: capture both presence and explicit-null sets so the validation
	// envelope can promote omitted/null required fields back to code=required
	// (per §1.2). The drop list is nil — Create has no read-only fields to
	// silently strip (that's a PATCH concern).
	explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(r, &request, nil)
	if err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	// Apply API-consumer defaults for fields the UI always sends explicitly
	// but API consumers commonly omit. Absence is distinguishable from zero
	// because these fields are pointer-typed.
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

	// TRA-514 / TRA-650 (BB23 F3): external_key is optional only by *omission*
	// — an absent key triggers the server auto-mint of ASSET-NNNN. When the
	// caller sends `external_key` explicitly, it must validate (min=1 +
	// pattern) to match the PATCH validator on UpdateAssetRequest.external_key.
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

	// TRA-681: location_id and location_external_key form a oneOf on Create
	// bodies — the spec encodes `not: {required: [location_id,
	// location_external_key]}` on CreateAssetWithTagsRequest. Reject both-
	// supplied at the handler so callers get a typed ambiguous_fields code
	// they can branch on, rather than relying on a silent server pick.
	_, hasLocID := presentKeys["location_id"]
	_, hasLocExt := presentKeys["location_external_key"]
	if hasLocID && hasLocExt {
		httputil.WriteValidationError(w, r, requestID, []modelerrors.FieldError{
			{Field: "location_id", Code: "ambiguous_fields", Message: "location_id and location_external_key are mutually exclusive; supply exactly one"},
			{Field: "location_external_key", Code: "ambiguous_fields", Message: "location_id and location_external_key are mutually exclusive; supply exactly one"},
		})
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationErrorWithPresence(w, r, err, requestID, presentKeys, explicitNulls)
		return
	}

	resolved, fErr := handler.resolveLocation(r, orgID, request.LocationID, request.LocationExternalKey)
	if fErr != nil {
		if fErr.Code == "internal_error" {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
				fErr.Message, requestID)

			return
		}
		// TRA-681: fk_not_found surfaces as 400 validation_error on both
		// surrogate and natural-key paths. Industry precedent (Stripe, AWS,
		// Atlassian) treats "your body references a row that does not exist"
		// as a validation failure rather than a state-conflict; 409 is
		// reserved for true state-conflict cases like the non-leaf-location-
		// delete pattern. The typed code stays so generated clients can
		// branch precisely.
		httputil.WriteValidationError(w, r, requestID, []modelerrors.FieldError{*fErr})

		return
	}
	request.LocationID = resolved

	request.OrgID = orgID

	result, err := handler.storage.CreateAssetWithTags(r.Context(), request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				err.Error(), requestID)

			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.Header().Set("Location", "/api/v1/assets/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": asset.ToPublicAssetView(*result)})
}

// @Summary      Update an asset
// @Description  Apply a JSON Merge Patch (RFC 7396) to an asset. Only fields included in the request body are changed; fields set to `null` clear the corresponding nullable column. Omitted fields are left unchanged. An empty body (`{}`) is a no-op and returns the current resource unchanged. Round-trip-safe server-owned fields (`id`, `created_at`, `updated_at`, `deleted_at`) are silently stripped from the body. The natural-key reference fields `external_key`, `location_id`, and `location_external_key` are read-only on PATCH: a value matching the current resource state is silently stripped (so a verbatim GET → PATCH round-trip succeeds), and a differing value returns 400 with `code: read_only` naming the correct write path. Mutate `external_key` via POST /assets/{asset_id}/rename; asset location is derived from scan events (record a scan event to change it); mutate `tags` via POST /assets/{asset_id}/tags and DELETE /assets/{asset_id}/tags/{tag_id}.
// @Tags         assets,public
// @ID           assets.update
// @Accept       json
// @Produce      json
// @Param        asset_id path  int                       true  "Asset id (canonical)" minimum(1) maximum(2147483647) format(int32)
// @Param        request  body  asset.UpdateAssetRequest  true  "Fields to merge-patch"
// @Success      200  {object}  assets.UpdateAssetResponse
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      415  {object}  modelerrors.ErrorResponse     "unsupported_media_type"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     BearerAuth[assets:write]
// @Router       /api/v1/assets/{asset_id} [patch]
func (handler *Handler) Update(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyAssetID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doUpdate(w, req, orgID, id)
}

func (handler *Handler) doUpdate(w http.ResponseWriter, req *http.Request, orgID, id int) {
	reqID := middleware.GetRequestID(req.Context())

	// TRA-686 / BB29 F7: reject `tags` pre-decode with 400 invalid_value —
	// tags are managed via the /assets/{id}/tags subresource. The
	// silent-drop default hid bugs in read-modify-write integrations.
	//
	// TRA-699 (BB31 §2): `external_key`, `location_id`, and
	// `location_external_key` are no longer on this reject list. They
	// follow the uniform accept-if-matches, reject-if-differs rule
	// implemented below after decode.
	if httputil.RejectFields(w, req, reqID, asset.PublicRejectPatchFields) {
		return
	}

	var request asset.UpdateAssetRequest
	// TRA-692: capture presentKeys alongside explicitNulls so the validator
	// response can promote any future length-bearing required violation to
	// code=required for the omitted/null cases. UpdateAssetRequest has no
	// `required` tags today, but threading presence here keeps the pattern
	// consistent with POST.
	explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, asset.PublicReadOnlyFields)
	if err != nil {
		httputil.RespondDecodeError(w, req, err, reqID)
		return
	}

	// valid_from / name are non-nullable on the read view; an explicit null
	// in the PATCH body is a validation error, not a clear-request.
	//
	// TRA-702 / BB32 D3: accumulate every null-on-non-nullable violation
	// before responding — the loop pre-TRA-702 short-circuited on the first
	// match, hiding subsequent ones until the integrator re-tried.
	var nullViolations []modelerrors.FieldError
	for _, f := range []string{"valid_from", "name", "is_active", "metadata"} {
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

	// TRA-699 (BB31 §2): natural-key echo check. Three fields are read-only
	// on PATCH but accept a verbatim echo of the current value as a silent
	// no-op so a GET → PATCH round-trip without an explicit strip succeeds.
	// A differing value is 400 read_only naming the dedicated write path.
	//
	//   external_key            → POST /assets/{asset_id}/rename
	//   location_id             → record a scan event (record-of-origin posture; TRA-411)
	//   location_external_key   → record a scan event
	current, err := handler.storage.GetAssetWithLocationByID(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)
		return
	}
	if current == nil {
		httputil.Respond404(w, req, apierrors.AssetNotFound, reqID)
		return
	}
	var echoViolations []modelerrors.FieldError
	if _, present := presentKeys["external_key"]; present {
		matched := request.ExternalKey != nil && *request.ExternalKey == current.ExternalKey
		if !matched {
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "external_key",
				Code:    "read_only",
				Message: "external_key is immutable via PATCH; use POST /api/v1/assets/{asset_id}/rename to change it",
			})
		}
	}
	if _, present := presentKeys["location_id"]; present {
		_, bodyNull := explicitNulls["location_id"]
		curNull := current.LocationID == nil
		matched := bodyNull && curNull
		if !bodyNull && !curNull && request.LocationID != nil && *request.LocationID == *current.LocationID {
			matched = true
		}
		if !matched {
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "location_id",
				Code:    "read_only",
				Message: "asset location is derived from scan events and not directly settable; record a scan event to update asset location",
			})
		}
	}
	if _, present := presentKeys["location_external_key"]; present {
		_, bodyNull := explicitNulls["location_external_key"]
		curNull := current.LocationExternalKey == nil
		matched := bodyNull && curNull
		if !bodyNull && !curNull && request.LocationExternalKey != nil && current.LocationExternalKey != nil &&
			*request.LocationExternalKey == *current.LocationExternalKey {
			matched = true
		}
		if !matched {
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "location_external_key",
				Code:    "read_only",
				Message: "asset location is derived from scan events and not directly settable; record a scan event to update asset location",
			})
		}
	}
	if len(echoViolations) > 0 {
		// TRA-702 / BB32 D2: detail must echo fields[0].Message — the inline
		// literal "validation failed" buried the redirect-to-/rename message
		// inside fields[0] where AI integrators were less likely to read it.
		httputil.WriteValidationError(w, req, reqID, echoViolations)
		return
	}
	// Echo passed (or fields absent). Strip the natural-key fields so they
	// don't reach storage as writable fields, and clear any presence/null
	// signals so the validator doesn't treat them as required violations.
	request.ExternalKey = nil
	request.LocationID = nil
	request.LocationExternalKey = nil
	delete(presentKeys, "external_key")
	delete(presentKeys, "location_id")
	delete(presentKeys, "location_external_key")
	delete(explicitNulls, "external_key")
	delete(explicitNulls, "location_id")
	delete(explicitNulls, "location_external_key")

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationErrorWithPresence(w, req, err, reqID, presentKeys, explicitNulls)
		return
	}

	result, err := handler.storage.UpdateAsset(req.Context(), orgID, id, request)
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
		httputil.Respond404(w, req, apierrors.AssetNotFound, reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": asset.ToPublicAssetView(*result)})
}

// @Summary      Delete an asset
// @Description  Delete an asset by its canonical id. The asset is removed from all subsequent queries and its external_key becomes immediately available for reuse. Returns 204 on success, 404 if the asset does not exist or has already been deleted.
// @Tags         assets,public
// @ID           assets.delete
// @Accept       json
// @Produce      json
// @Param        asset_id  path  int  true  "Asset id (canonical)" minimum(1) maximum(2147483647) format(int32)
// @Success      204  "deleted"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     BearerAuth[assets:write]
// @Router       /api/v1/assets/{asset_id} [delete]
func (handler *Handler) Delete(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyAssetID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doDelete(w, req, orgID, id)
}

func (handler *Handler) doDelete(w http.ResponseWriter, req *http.Request, orgID, id int) {
	reqID := middleware.GetRequestID(req.Context())

	deleted, err := handler.storage.DeleteAsset(req.Context(), orgID, id)
	if err != nil {
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}

	if !deleted {
		httputil.Respond404(w, req, apierrors.AssetNotFound, reqID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListAssetsResponse is the typed envelope returned by GET /api/v1/assets.
type ListAssetsResponse struct {
	Data       []asset.PublicAssetView `json:"data"`
	Limit      int                     `json:"limit"       example:"50"`
	Offset     int                     `json:"offset"      example:"0"`
	TotalCount int                     `json:"total_count" example:"100"`
}

// GetAssetResponse is the typed envelope returned by GET /api/v1/assets/{asset_id}.
type GetAssetResponse struct {
	Data asset.PublicAssetView `json:"data"`
}

// CreateAssetResponse is the typed envelope returned by POST /api/v1/assets.
type CreateAssetResponse struct {
	Data asset.PublicAssetView `json:"data"`
}

// UpdateAssetResponse is the typed envelope returned by PATCH /api/v1/assets/{asset_id}.
type UpdateAssetResponse struct {
	Data asset.PublicAssetView `json:"data"`
}

// RenameAssetResponse is the typed envelope returned by POST /api/v1/assets/{asset_id}/rename.
// TRA-664.
type RenameAssetResponse struct {
	Data asset.PublicAssetView `json:"data"`
}

// @Summary      Rename an asset (mutate external_key)
// @Description  **Required scope:** `assets:write`
// @Description
// @Description  Mutate the asset's `external_key` (natural / join key). This operation is **destructive to downstream joins**: any external system that has cached or indexed records on the old `external_key` will silently disconnect. Prefer a coordinated cutover with downstream consumers.
// @Description
// @Description  `external_key` is immutable via PATCH; this operation is the only way to change it. Distinct from a regular PATCH in audit logs (different URL surface).
// @Tags         assets,public
// @ID           assets.rename
// @Accept       json
// @Produce      json
// @Param        asset_id path  int                      true  "Asset id (canonical)" minimum(1) maximum(2147483647) format(int32)
// @Param        request  body  asset.RenameAssetRequest true  "New external_key"
// @Success      200  {object}  assets.RenameAssetResponse
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      415  {object}  modelerrors.ErrorResponse     "unsupported_media_type"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     BearerAuth[assets:write]
// @Router       /api/v1/assets/{asset_id}/rename [post]
func (handler *Handler) Rename(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, ok := handler.parseAndVerifyAssetID(w, req, orgID, reqID)
	if !ok {
		return
	}

	var request asset.RenameAssetRequest
	// TRA-692: swap to a presence-tracking decoder so an omitted or null
	// `external_key` surfaces as code=required (not the TRA-675-collapsed
	// too_short). The body has a single required string field, so this is
	// exactly the §1.2 case.
	explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, nil)
	if err != nil {
		httputil.RespondDecodeError(w, req, err, reqID)
		return
	}
	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationErrorWithPresence(w, req, err, reqID, presentKeys, explicitNulls)
		return
	}

	result, err := handler.storage.RenameAsset(req.Context(), orgID, id, request.ExternalKey)
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
		httputil.Respond404(w, req, apierrors.AssetNotFound, reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": asset.ToPublicAssetView(*result)})
}

// @Summary List assets
// @Description Paginated assets list with natural-key filters, sort, and substring search.
// @Description
// @Description Default scope returns currently-effective assets only — rows whose `valid_from` is in the past AND whose `valid_to` is null or in the future. The `is_active` filter is independent of temporal validity; omit it to include both active and inactive rows within the effective window, or pass `?is_active=true`/`false` to filter further.
// @Tags assets,public
// @ID assets.list
// @Accept json
// @Produce json
// @Param limit                 query int    false "max 200"   default(50) minimum(1) maximum(200)
// @Param offset                query int    false "min 0"     default(0) minimum(0)
// @Param location_id           query []int    false "filter by current location id (canonical, may repeat); mutually exclusive with location_external_key (400 ambiguous_fields if both supplied)" collectionFormat(multi)
// @Param location_external_key query []string false "filter by current location external_key (may repeat); mutually exclusive with location_id (400 ambiguous_fields if both supplied)" collectionFormat(multi)
// @Param external_key          query []string false "filter by asset external_key, equality match (may repeat for any-of)" collectionFormat(multi)
// @Param is_active             query bool   false "filter by active flag"
// @Param include_deleted       query bool   false "when true, include soft-deleted rows in the response. deleted_at is populated for those rows. Orthogonal to is_active." default(false)
// @Param q                     query string false "substring search (case-insensitive) on name, external_key, description, and active tag values"
// @Param sort                  query []string false "comma-separated; prefix '-' for DESC" collectionFormat(csv) Enums(external_key, -external_key, name, -name, created_at, -created_at, updated_at, -updated_at)
// @Success 200 {object} assets.ListAssetsResponse
// @Header  200 {integer} X-RateLimit-Limit     "Steady-state requests/min for this API key"
// @Header  200 {integer} X-RateLimit-Remaining "Requests remaining before throttling; bounded by X-RateLimit-Limit"
// @Header  200 {integer} X-RateLimit-Reset     "Unix timestamp (seconds) when X-RateLimit-Remaining will next equal X-RateLimit-Limit"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Header  429 {integer} Retry-After           "Seconds to wait before retrying"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth[assets:read]
// @Router /api/v1/assets [get]
func (handler *Handler) ListAssets(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Filters:     []string{"location_id", "location_external_key", "external_key", "is_active", "include_deleted", "q"},
		BoolFilters: []string{"is_active", "include_deleted"},
		Sorts:       []string{"external_key", "name", "created_at", "updated_at"},
	})
	if err != nil {
		httputil.RespondListParamError(w, req, err, reqID)
		return
	}

	// TRA-681: location_id and location_external_key form a oneOf on the
	// GET filter — reject 400 ambiguous_fields when both are supplied so
	// integrators get a typed signal rather than a silent winner. OpenAPI 3
	// can't encode a query-parameter pair constraint, so the rule lives in
	// the per-parameter description and the handler validation here.
	_, hasLocID := params.Filters["location_id"]
	_, hasLocExt := params.Filters["location_external_key"]
	if hasLocID && hasLocExt {
		httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{
			{Field: "location_id", Code: "ambiguous_fields", Message: "location_id and location_external_key are mutually exclusive; supply exactly one"},
			{Field: "location_external_key", Code: "ambiguous_fields", Message: "location_id and location_external_key are mutually exclusive; supply exactly one"},
		})
		return
	}
	f := asset.ListFilter{
		LocationExternalKeys: params.Filters["location_external_key"],
		ExternalKeys:         params.Filters["external_key"],
		Limit:                params.Limit,
		Offset:               params.Offset,
	}
	if vs, ok := params.Filters["location_id"]; ok && len(vs) > 0 {
		f.LocationIDs = make([]int, 0, len(vs))
		for _, s := range vs {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || int64(n) > httputil.SurrogateIDMax {
				httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{{
					Field:   "location_id",
					Code:    "invalid_value",
					Message: fmt.Sprintf("location_id %q must be a positive int32", s),
				}})

				return
			}
			f.LocationIDs = append(f.LocationIDs, n)
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
		f.Sorts = append(f.Sorts, asset.ListSort{Field: s.Field, Desc: s.Desc})
	}

	items, err := handler.storage.ListAssetsFiltered(req.Context(), orgID, f)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	total, err := handler.storage.CountAssetsFiltered(req.Context(), orgID, f)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	out := make([]asset.PublicAssetView, 0, len(items))
	for _, a := range items {
		out = append(out, asset.ToPublicAssetView(a))
	}

	httputil.WriteJSON(w, http.StatusOK, ListAssetsResponse{
		Data:       out,
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}

// @Summary Get asset by canonical id
// @Description Retrieve an asset by its canonical id. Returns 404 if the asset does not exist.
// @Description
// @Description Path-addressed retrieval bypasses the temporal-validity filter applied on list endpoints — any non-deleted asset is returned regardless of its `valid_from` / `valid_to` values. Use this endpoint when you have an id and need the row even if its effective window has elapsed.
// @Tags assets,public
// @ID assets.get
// @Param asset_id path int true "Asset id (canonical)" minimum(1) maximum(2147483647) format(int32)
// @Success 200 {object} assets.GetAssetResponse
// @Header  200 {integer} X-RateLimit-Limit     "Steady-state requests/min for this API key"
// @Header  200 {integer} X-RateLimit-Remaining "Requests remaining before throttling; bounded by X-RateLimit-Limit"
// @Header  200 {integer} X-RateLimit-Reset     "Unix timestamp (seconds) when X-RateLimit-Remaining will next equal X-RateLimit-Limit"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Header  429 {integer} Retry-After           "Seconds to wait before retrying"
// @Security BearerAuth[assets:read]
// @Router /api/v1/assets/{asset_id} [get]
func (handler *Handler) GetAsset(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.RespondMissingOrgContext(w, req, reqID)
		return
	}

	id, err := httputil.ParseSurrogateID("asset_id", chi.URLParam(req, "asset_id"))
	if err != nil {
		httputil.RespondPathParamError(w, req, err, reqID)
		return
	}

	view, err := handler.storage.GetAssetWithLocationByID(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}
	if view == nil {
		httputil.Respond404(w, req, apierrors.AssetNotFound, reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data": asset.ToPublicAssetView(*view),
	})
}

// AddTagResponse is the typed envelope returned by POST /api/v1/assets/{asset_id}/tags.
type AddTagResponse struct {
	Data shared.Tag `json:"data"`
}

// @Summary      Add a tag to an asset
// @Description  Attach a tag (RFID EPC, BLE beacon ID, barcode, etc.) to an existing asset.
// @Description  The tag must be unique within the organization.
// @Tags         assets,public
// @ID           assets.tags.add
// @Accept       json
// @Produce      json
// @Param        asset_id path  int                true  "Asset id (canonical)" minimum(1) maximum(2147483647) format(int32)
// @Param        request  body  shared.TagRequest  true  "Tag to attach"
// @Success      201  {object}  assets.AddTagResponse         "tag attached"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      415  {object}  modelerrors.ErrorResponse     "unsupported_media_type"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     BearerAuth[assets:write]
// @Router       /api/v1/assets/{asset_id}/tags [post]
func (handler *Handler) AddTag(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	id, ok := handler.parseAndVerifyAssetID(w, r, orgID, requestID)
	if !ok {
		return
	}

	handler.doAddAssetTag(w, r, orgID, id)
}

// doAddAssetTag decodes the tag body, validates it, and inserts via storage.
// Caller must have already verified that (orgID, assetID) names a real asset
// — storage.AddTagToAsset does NOT cross-check ownership before INSERT, so
// skipping the pre-check would allow cross-org tag attachment.
func (handler *Handler) doAddAssetTag(w http.ResponseWriter, r *http.Request, orgID, assetID int) {
	requestID := middleware.GetRequestID(r.Context())

	var request shared.TagRequest
	// TRA-692: presence-tracking decoder so an omitted `value` surfaces as
	// code=required (the TRA-675 collapse to too_short doesn't match the
	// §1.2 contract for a missing key).
	explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(r, &request, nil)
	if err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationErrorWithPresence(w, r, err, requestID, presentKeys, explicitNulls)
		return
	}

	tag, err := handler.storage.AddTagToAsset(r.Context(), orgID, assetID, request)
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

// @Summary      Remove a tag from an asset
// @Description  Detach a tag from an asset by its tag record id.
// @Description  Idempotent: returns 204 whether or not the tag was associated. Repeated calls are safe.
// @Tags         assets,public
// @ID           assets.tags.remove
// @Accept       json
// @Produce      json
// @Param        asset_id  path  int  true  "Asset id (canonical)" minimum(1) maximum(2147483647) format(int32)
// @Param        tag_id    path  int  true  "Tag id" minimum(1) maximum(2147483647) format(int32)
// @Success      204  "deleted"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     BearerAuth[assets:write]
// @Router       /api/v1/assets/{asset_id}/tags/{tag_id} [delete]
func (handler *Handler) RemoveTag(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	id, err := httputil.ParseSurrogateID("asset_id", chi.URLParam(r, "asset_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, requestID)
		return
	}

	a, err := handler.storage.GetAssetByID(r.Context(), orgID, &id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), requestID)

		return
	}
	if a == nil || a.OrgID != orgID {
		httputil.Respond404(w, r, apierrors.AssetNotFound, requestID)
		return
	}

	handler.doRemoveAssetTag(w, r, orgID, a.ID)
}

// doRemoveAssetTag parses {tag_id} and soft-deletes via storage.
// Storage guards cross-asset / cross-org misuse itself (EXISTS subquery on
// asset_id + org_id), so a missing match surfaces as deleted=false rather
// than an error.
func (handler *Handler) doRemoveAssetTag(w http.ResponseWriter, r *http.Request, orgID, assetID int) {
	requestID := middleware.GetRequestID(r.Context())

	tagID, err := httputil.ParseSurrogateID("tag_id", chi.URLParam(r, "tag_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, requestID)
		return
	}

	_, err = handler.storage.RemoveAssetTag(r.Context(), orgID, assetID, tagID)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// parseAndVerifyAssetID extracts {asset_id}, parses it as a surrogate int,
// and verifies the asset exists and belongs to the caller's org. Writes an
// appropriate 400 / 404 / 500 response and returns ok=false on any failure.
func (handler *Handler) parseAndVerifyAssetID(w http.ResponseWriter, req *http.Request, orgID int, reqID string) (int, bool) {
	id, err := httputil.ParseSurrogateID("asset_id", chi.URLParam(req, "asset_id"))
	if err != nil {
		httputil.RespondPathParamError(w, req, err, reqID)
		return 0, false
	}

	a, err := handler.storage.GetAssetByID(req.Context(), orgID, &id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return 0, false
	}
	if a == nil || a.OrgID != orgID {
		httputil.Respond404(w, req, apierrors.AssetNotFound, reqID)
		return 0, false
	}

	return a.ID, true
}

// RegisterRoutes keeps only session-only surface (bulk CSV). Public read,
// write, and lookup routes are registered directly in
// internal/cmd/serve/router.go under EitherAuth.
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/assets/bulk", handler.UploadCSV)
	r.Get("/api/v1/assets/bulk/{jobId}", handler.GetJobStatus)
}
