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

// TRA-734 (BB40 F3): location_id / location_external_key on a POST or PATCH
// body are rejected with code=read_only. The error detail honestly describes
// the master-data / scan-data bifurcation and points at the consumption
// surfaces (GET /assets/{id}, GET /assets/{id}/history, GET /reports/asset-
// locations) rather than the previous opaque "record a scan event" line.
// Asset location is collected through ingestion paths separate from the
// public API: the fixed-reader MQTT pipeline and handheld UI submission.
//
// TRA-750 / BB46 F3: the trailing "See https://docs.trakrf.id/..." link
// was dropped to avoid an env leak — a preview integrator hitting this
// error otherwise gets routed to production docs. The error envelope's
// `type` field is the canonical documented entry point per /docs/api/
// errors; the trailing URL was redundant.
const assetLocationReadOnlyMessage = "asset location is collected through scan event ingestion (fixed-reader MQTT pipeline or handheld UI submission) and is not directly settable through the public API. Read current asset location through GET /api/v1/assets/{id}, GET /api/v1/assets/{id}/history, or GET /api/v1/reports/asset-locations."

// PublicRejectCreateFields names the JSON keys that POST /api/v1/assets
// rejects pre-decode with 400 validation_error and code=read_only. Same
// shape as PublicRejectPatchFields on the PATCH side. TRA-734 (BB40 F3):
// location_id and location_external_key are scan/operational data —
// they are derived from ingestion, never set by an integrator on the
// public API.
var PublicRejectCreateFields = map[string]httputil.FieldRejectPolicy{
	"location_id":           {Code: "read_only", Message: assetLocationReadOnlyMessage},
	"location_external_key": {Code: "read_only", Message: assetLocationReadOnlyMessage},
}

// @Summary      Create an asset
// @Description  Create a new asset record, optionally with one or more tags (RFID, BLE, barcode).
// @Description
// @Description  The `external_key` field is optional. Provide a value from your system of record
// @Description  (ERP, WMS, asset management) for natural-key joins, or omit it to receive a
// @Description  server-assigned external_key in the format `ASSET-NNNN` (per-organization sequence).
// @Description  A caller-supplied external_key that collides with an existing asset returns 409.
// @Description
// @Description  Returns the created asset with its assigned tags. The Location response header contains the path of the created resource (resolve against the request URL per RFC 7231 §7.1.2).
// @Tags         assets,public
// @ID           assets.create
// @Accept       json
// @Produce      json
// @Param        request  body  asset.CreateAssetWithTagsRequest  true  "Asset to create with optional tags"
// @Success      201  {object}  assets.CreateAssetResponse
// @Header       201  {string}  Location  "Path of the created resource (resolve against request URL per RFC 7231 §7.1.2)"
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

	// TRA-734 (BB40 F3): asset location is scan/operational data and not
	// directly settable through the public API. Reject location_id /
	// location_external_key pre-decode with code=read_only and a detail that
	// names the consumption paths. The struct fields are absent on
	// CreateAssetRequest, so without this pre-decode reject a caller would
	// see the generic unknown_field code instead of the positioning-coherent
	// read_only message.
	if httputil.RejectFields(w, r, requestID, PublicRejectCreateFields) {
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

	// TRA-705 (BB32 §C6) + TRA-732 R2: every non-nullable field — including
	// the required string fields `name` and `external_key` — emits
	// `invalid_value` for explicit null. `required` is reserved for the
	// absent-key case; explicit null on a non-nullable field is "you sent
	// a bad value," not "you forgot to include this." Accumulate every
	// violation in one response (BB32 §D3 pattern).
	var nullViolations []modelerrors.FieldError
	for _, f := range []string{"valid_from", "is_active", "metadata"} {
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

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationErrorWithPresence(w, r, err, requestID, presentKeys, explicitNulls)
		return
	}

	// TRA-765 (BB56 F3): reject inverted or instantaneous validity windows.
	// valid_from has been defaulted to time.Now() above when absent, so the
	// comparison runs against an effective non-zero value.
	var assetValidTo *time.Time
	if request.ValidTo != nil {
		t := request.ValidTo.ToTime()
		assetValidTo = &t
	}
	if fe := httputil.ValidateValidityWindow(request.ValidFrom.ToTime(), assetValidTo); fe != nil {
		httputil.WriteValidationError(w, r, requestID, []modelerrors.FieldError{*fe})
		return
	}

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
// @Description  Apply a JSON Merge Patch (RFC 7396) to an asset. Only fields included in the request body are changed; fields set to `null` clear the corresponding nullable column. Omitted fields are left unchanged. Every accepted PATCH — empty body (`{}`), verbatim echo of current values, partial mutation, or full mutation — advances `updated_at` on success (filesystem `touch` semantics). Read-only fields are uniformly governed by the accept-if-matches, reject-if-differs rule: a value matching the current resource state is silently normalized out (so a verbatim GET → PATCH round-trip succeeds without manual scrubbing), and a differing value returns 400. The rejection `code` splits the two semantic classes: server-managed fields (`id`, `created_at`, `updated_at`, `deleted_at`, `location_id`, `location_external_key`) return `code: read_only` — they have no public mutation path. Fields mutable via a sub-resource verb (`external_key`, `tags`) return `code: invalid_context` and the detail names the correct verb: mutate `external_key` via POST /assets/{asset_id}/rename; mutate `tags` via POST /assets/{asset_id}/tags and DELETE /assets/{asset_id}/tags/{tag_id}. The `tags` collection is compared as a set on full tag content — array ordering is not significant; differing set membership or differing field values on a matching id returns 400 `invalid_context`. Asset location is collected through scan event ingestion (fixed-reader MQTT pipeline or handheld UI submission) and is not directly settable through the public API.
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

	// TRA-710 (BB33 F2): server-managed read-only fields (id, created_at,
	// updated_at, deleted_at, tags) follow the accept-if-matches /
	// reject-if-differs rule. Peek raw body values before strict decode
	// strips id/created_at/updated_at/deleted_at and before the
	// UpdateAssetRequest decode silently ignores tags. Compared against
	// current resource state below.
	rawReadOnly := httputil.PeekJSONFields(req, []string{
		"id", "created_at", "updated_at", "deleted_at", "tags",
	})

	// PublicRejectPatchFields is currently empty (tags moved to the echo
	// check under TRA-710); kept as the call site for future fields whose
	// mere presence is invalid regardless of value.
	if httputil.RejectFields(w, req, reqID, asset.PublicRejectPatchFields) {
		return
	}

	var request asset.UpdateAssetRequest
	// TRA-692: capture presentKeys alongside explicitNulls so the validator
	// response can promote any future length-bearing required violation to
	// code=required for the omitted/null cases. UpdateAssetRequest has no
	// `required` tags today, but threading presence here keeps the pattern
	// consistent with POST.
	//
	// TRA-710: `tags` is added to the drop list alongside PublicReadOnlyFields
	// because UpdateAssetRequest has no `tags` field to decode into; the echo
	// check above already captured its raw value.
	dropFields := append([]string{"tags"}, asset.PublicReadOnlyFields...)
	explicitNulls, presentKeys, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(req, &request, dropFields)
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
	//   location_id             → scan event ingestion (TRA-734 / BB40 F3)
	//   location_external_key   → scan event ingestion (TRA-734 / BB40 F3)
	//
	// TRA-710 (BB33 F2): the same accept-if-matches / reject-if-differs
	// rule covers the server-managed surrogate id and timestamps (id,
	// created_at, updated_at, deleted_at) and the `tags` collection. Their
	// raw body values were peeked above (rawReadOnly) so the comparison
	// here runs against the current resource state.
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
	// TRA-710: server-managed read-only echo checks. The current view is
	// projected through ToPublicAssetView so the wire shape we compare
	// against matches the wire shape an integrator GETs.
	currentView := asset.ToPublicAssetView(*current)
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
				Message: "deleted_at is server-managed; use DELETE /api/v1/assets/{asset_id} to soft-delete. Submit the resource's current deleted_at or omit the field",
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
			// TRA-780 F4: `tags` is mutable on the asset surface — just via
			// POST/DELETE on the /tags sub-resource, not via PATCH. Emit
			// `invalid_context` (wrong verb for this field) rather than
			// `read_only` (truly server-managed) so strict-typed clients
			// branching on code can route the integrator to the correct
			// verb. Detail string is unchanged.
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "tags",
				Code:    "invalid_context",
				Message: "the tags field on PATCH must equal the current value as a set (idempotent echo only; ordering is not significant); use POST /api/v1/assets/{asset_id}/tags and DELETE /api/v1/assets/{asset_id}/tags/{tag_id} to mutate",
			})
		}
	}
	if _, present := presentKeys["external_key"]; present {
		matched := request.ExternalKey != nil && *request.ExternalKey == current.ExternalKey
		if !matched {
			// TRA-780 F4: external_key is mutable via POST /rename — emit
			// `invalid_context` rather than `read_only` so the code matches
			// the semantic ("wrong verb for this field"). Detail string is
			// unchanged.
			echoViolations = append(echoViolations, modelerrors.FieldError{
				Field:   "external_key",
				Code:    "invalid_context",
				Message: `external_key is immutable via PATCH; use POST /api/v1/assets/{asset_id}/rename with body {"external_key": "<new value>"} to change it`,
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
				Message: assetLocationReadOnlyMessage,
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
				Message: assetLocationReadOnlyMessage,
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

	// TRA-765 (BB56 F3): reject inverted or instantaneous validity windows on
	// PATCH. Effective valid_from is the body value when supplied else the
	// current value; effective valid_to is nil when the body clears it,
	// the body value when present and non-null, else the current value.
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
//
// TRA-719 / BB35 B3: includes `descendant_count_affected` for shape
// uniformity with RenameLocationResponse. Assets have no hierarchy, so
// the value is always 0; surfacing the field saves typed-client consumers
// from branching on rename-verb when they emit a "subtree may need
// refresh" hint.
type RenameAssetResponse struct {
	Data                    asset.PublicAssetView `json:"data"`
	DescendantCountAffected int                   `json:"descendant_count_affected" example:"0"`
}

// @Summary      Rename an asset (mutate external_key)
// @Description  **Required scope:** `assets:write`
// @Description
// @Description  Mutate the asset's `external_key` (natural / join key). This operation is **destructive to downstream joins**: any external system that has cached or indexed records on the old `external_key` will silently disconnect. Prefer a coordinated cutover with downstream consumers.
// @Description
// @Description  `external_key` is immutable via PATCH; this operation is the only way to change it. Distinct from a regular PATCH in audit logs (different URL surface).
// @Description
// @Description  The response includes `descendant_count_affected` for shape uniformity with the location rename verb. Assets have no hierarchy, so this value is always 0; it is emitted to save typed-client consumers from branching on rename verb.
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

	// TRA-719 / BB35 B3: emit descendant_count_affected=0 for shape
	// uniformity with RenameLocationResponse. Assets have no hierarchy.
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":                      asset.ToPublicAssetView(*result),
		"descendant_count_affected": 0,
	})
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

	// TRA-713 / BB33 F5+C2: external_key-style filters must enforce the
	// same regex the field validators apply on POST/PATCH. Without this,
	// a slash-containing (or otherwise non-conforming) value silently
	// returns 200-with-empty rather than 400 invalid_value, masking
	// integration bugs at the boundary.
	if fe := httputil.ValidateExternalKeyFilterValues("external_key", params.Filters["external_key"]); fe != nil {
		httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{*fe})
		return
	}
	if fe := httputil.ValidateExternalKeyFilterValues("location_external_key", params.Filters["location_external_key"]); fe != nil {
		httputil.WriteValidationError(w, req, reqID, []modelerrors.FieldError{*fe})
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
					Message: fmt.Sprintf("location_id %q must be a positive integer ≤ %d", s, httputil.SurrogateIDMax),
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
// @Failure 500 {object} modelerrors.ErrorResponse
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
// @Header       201  {string}  Location                      "Path of the created tag (resolve against request URL per RFC 7231 §7.1.2)"
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

	// TRA-707 / BB32 C2: emit Location pointing at the newly created tag
	// subresource (RFC 7231 §7.1.2). Matches the canonical-URL pattern on
	// POST /api/v1/assets.
	w.Header().Set("Location", fmt.Sprintf("/api/v1/assets/%d/tags/%d", assetID, tag.ID))
	httputil.WriteJSON(w, http.StatusCreated, AddTagResponse{Data: *tag})
}

// @Summary      Remove a tag from an asset
// @Description  Detach a tag from an asset by its tag record id.
// @Description  First successful removal returns 204; repeated calls return 404 — consistent with top-level resource DELETE semantics. The cross-asset / cross-org case (a tag that exists but is not attached to this asset, or belongs to a different org) also surfaces as 404.
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

	removed, err := handler.storage.RemoveAssetTag(r.Context(), orgID, assetID, tagID)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}
	// TRA-719 / BB35 A3: align tag subresource DELETE with top-level
	// DELETE semantics — second call returns 404, not 204. The cross-
	// asset and cross-org cases also fall here (storage guard returns
	// removed=false rather than an error).
	if !removed {
		httputil.Respond404(w, r, "Tag not found on this asset", requestID)
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
