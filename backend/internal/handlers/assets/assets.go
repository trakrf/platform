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

// @Summary      Create an asset
// @Description  Create a new asset record, optionally with one or more tag identifiers (RFID, BLE, barcode).
// @Description  Returns the created asset with its assigned identifiers. The Location response header contains the canonical URL.
// @Tags         assets,public
// @ID           assets.create
// @Accept       json
// @Produce      json
// @Param        request  body  asset.CreateAssetWithIdentifiersRequest  true  "Asset to create with optional identifiers"
// @Success      201  {object}  assets.CreateAssetResponse
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[assets:write]
// @Router       /api/v1/assets [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetCreateFailed, "missing organization context", requestID)
		return
	}

	var request asset.CreateAssetWithIdentifiersRequest
	if err := httputil.DecodeJSONStrict(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	// Apply API-consumer defaults for fields the UI always sends explicitly
	// but API consumers commonly omit. Absence is distinguishable from zero
	// because these fields are pointer-typed.
	if request.Type == "" {
		request.Type = "asset"
	}
	if request.IsActive == nil {
		t := true
		request.IsActive = &t
	}
	if request.ValidFrom == nil || request.ValidFrom.IsZero() {
		fd := shared.FlexibleDate{Time: time.Now().UTC()}
		request.ValidFrom = &fd
	}

	// Resolve current_location → current_location_id (TRA-477). Empty string
	// is treated as nil. Parallels parent_identifier handling on locations.
	if request.CurrentLocation != nil && *request.CurrentLocation != "" {
		loc, err := handler.storage.GetLocationByIdentifier(r.Context(), orgID, *request.CurrentLocation)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
				apierrors.AssetCreateFailed, err.Error(), requestID)
			return
		}
		if loc == nil {
			msg := fmt.Sprintf("current_location %q not found", *request.CurrentLocation)
			httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
				apierrors.AssetCreateFailed, msg, requestID,
				[]modelerrors.FieldError{{
					Field:   "current_location",
					Code:    "invalid_value",
					Message: msg,
				}})
			return
		}
		if request.CurrentLocationID != nil && *request.CurrentLocationID != loc.ID {
			msg := "current_location and current_location_id disagree"
			httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
				apierrors.AssetCreateFailed, msg, requestID,
				[]modelerrors.FieldError{{
					Field:   "current_location",
					Code:    "invalid_value",
					Message: msg,
				}})
			return
		}
		request.CurrentLocationID = &loc.ID
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	request.OrgID = orgID

	result, err := handler.storage.CreateAssetWithIdentifiers(r.Context(), request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.AssetCreateFailed, err.Error(), requestID)
			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.Header().Set("Location", "/api/v1/assets/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": asset.ToPublicAssetView(*result)})
}

// @Summary      Update an asset
// @Description  Update mutable fields on an existing asset. Only fields included in the request body are changed.
// @Tags         assets,public
// @ID           assets.update
// @Accept       json
// @Produce      json
// @Param        identifier  path  string                      true  "Asset identifier"
// @Param        request     body  asset.UpdateAssetRequest    true  "Fields to update"
// @Success      200  {object}  assets.UpdateAssetResponse
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[assets:write]
// @Router       /api/v1/assets/{identifier} [put]
func (handler *Handler) UpdateAsset(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetUpdateFailed, "missing organization context", ctx)
		return
	}

	identifier := chi.URLParam(req, "identifier")

	a, err := handler.storage.GetAssetByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetGetFailed, err.Error(), ctx)
		return
	}
	if a == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", ctx)
		return
	}

	handler.doUpdateAsset(w, req, orgID, a.ID)
}

// doUpdateAsset contains the body-decode + validate + storage update logic
// shared between UpdateAsset (public, identifier-keyed) and UpdateAssetByID
// (internal, surrogate-keyed). Caller must have already verified that
// (orgID, id) names a real asset.
func (handler *Handler) doUpdateAsset(w http.ResponseWriter, req *http.Request, orgID, id int) {
	reqID := middleware.GetRequestID(req.Context())

	var request asset.UpdateAssetRequest
	explicitNulls, err := httputil.DecodeJSONStrictWithNulls(req, &request)
	if err != nil {
		httputil.RespondDecodeError(w, req, err, reqID)
		return
	}

	// valid_from is NOT NULL in the DB; explicit null is invalid. Explicit
	// valid_to null requests a clear (SQL NULL), per TRA-468 wire convention.
	if _, ok := explicitNulls["valid_from"]; ok {
		httputil.WriteJSONErrorWithFields(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.AssetUpdateFailed, "validation failed", reqID,
			[]modelerrors.FieldError{{
				Field:   "valid_from",
				Code:    "invalid_value",
				Message: "valid_from cannot be null; omit the field to leave unchanged, or provide a date",
			}})
		return
	}
	if _, ok := explicitNulls["valid_to"]; ok {
		request.ClearValidTo = true
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, req, err, reqID)
		return
	}

	// Resolve current_location → current_location_id (TRA-477).
	if request.CurrentLocation != nil && *request.CurrentLocation != "" {
		loc, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, *request.CurrentLocation)
		if err != nil {
			httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
				apierrors.AssetUpdateFailed, err.Error(), reqID)
			return
		}
		if loc == nil {
			msg := fmt.Sprintf("current_location %q not found", *request.CurrentLocation)
			httputil.WriteJSONErrorWithFields(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
				apierrors.AssetUpdateFailed, msg, reqID,
				[]modelerrors.FieldError{{
					Field:   "current_location",
					Code:    "invalid_value",
					Message: msg,
				}})
			return
		}
		if request.CurrentLocationID != nil && *request.CurrentLocationID != loc.ID {
			msg := "current_location and current_location_id disagree"
			httputil.WriteJSONErrorWithFields(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
				apierrors.AssetUpdateFailed, msg, reqID,
				[]modelerrors.FieldError{{
					Field:   "current_location",
					Code:    "invalid_value",
					Message: msg,
				}})
			return
		}
		request.CurrentLocationID = &loc.ID
	}

	result, err := handler.storage.UpdateAsset(req.Context(), orgID, id, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.AssetUpdateFailed, err.Error(), reqID)
			return
		}
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": asset.ToPublicAssetView(*result)})
}

// @Summary Get asset by surrogate ID (internal)
// @Description Session-auth-only variant for the frontend. Public consumers should use {identifier}.
// @Tags assets,internal
// @Param id path int true "Asset surrogate ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/assets/by-id/{id} [get]
func (handler *Handler) GetAssetByID(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetGetFailed, "missing organization context", reqID)
		return
	}

	idParam := chi.URLParam(req, "id")
	id, err := httputil.ParseSurrogateID(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.AssetGetInvalidID, idParam), err.Error(), reqID)
		return
	}

	view, err := handler.storage.GetAssetViewByID(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetGetFailed, err.Error(), reqID)
		return
	}
	if view == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", reqID)
		return
	}

	public := asset.ToPublicAssetView(asset.AssetWithLocation{
		AssetView: *view,
	})

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": public})
}

// @Summary      Delete an asset
// @Description  Delete an asset by its natural identifier. The asset is removed from all subsequent queries and its identifier becomes immediately available for reuse. Returns 204 on success, 404 if the asset does not exist or has already been deleted.
// @Tags         assets,public
// @ID           assets.delete
// @Accept       json
// @Produce      json
// @Param        identifier  path  string  true  "Asset identifier"
// @Success      204  "deleted"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[assets:write]
// @Router       /api/v1/assets/{identifier} [delete]
func (handler *Handler) DeleteAsset(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetDeleteFailed, "missing organization context", ctx)
		return
	}

	identifier := chi.URLParam(req, "identifier")

	a, err := handler.storage.GetAssetByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetGetFailed, err.Error(), ctx)
		return
	}
	if a == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", ctx)
		return
	}

	handler.doDeleteAsset(w, req, orgID, a.ID)
}

// doDeleteAsset performs the soft-delete against storage. Caller must have
// already verified that (orgID, id) names a real asset.
func (handler *Handler) doDeleteAsset(w http.ResponseWriter, req *http.Request, orgID, id int) {
	reqID := middleware.GetRequestID(req.Context())

	deleted, err := handler.storage.DeleteAsset(req.Context(), orgID, id)
	if err != nil {
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}

	if !deleted {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", reqID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListAssetsResponse is the typed envelope returned by GET /api/v1/assets.
// Shape mirrors the runtime map literal at the bottom of ListAssets.
type ListAssetsResponse struct {
	Data       []asset.PublicAssetView `json:"data"`
	Limit      int                     `json:"limit"       example:"50"`
	Offset     int                     `json:"offset"      example:"0"`
	TotalCount int                     `json:"total_count" example:"100"`
}

// GetAssetResponse is the typed envelope returned by GET /api/v1/assets/{identifier}.
type GetAssetResponse struct {
	Data asset.PublicAssetView `json:"data"`
}

// CreateAssetResponse is the typed envelope returned by POST /api/v1/assets.
type CreateAssetResponse struct {
	Data asset.PublicAssetView `json:"data"`
}

// UpdateAssetResponse is the typed envelope returned by PUT /api/v1/assets/{identifier}
// and PUT /api/v1/assets/by-id/{id}.
type UpdateAssetResponse struct {
	Data asset.PublicAssetView `json:"data"`
}

// @Summary List assets
// @Description Paginated assets list with natural-key filters, sort, and substring search
// @Tags assets,public
// @ID assets.list
// @Accept json
// @Produce json
// @Param limit    query int    false "max 200"   default(50)
// @Param offset   query int    false "min 0"    default(0)
// @Param location query string false "filter by location natural key (may repeat)"
// @Param is_active query bool  false "filter by active flag"
// @Param type     query string false "filter by type"
// @Param q        query string false "substring search (case-insensitive) on name, identifier, description, and active identifier values"
// @Param sort     query string false "comma-separated; prefix '-' for DESC"
// @Success 200 {object} assets.ListAssetsResponse
// @Header  200 {integer} X-RateLimit-Limit     "Steady-state requests/min for this API key"
// @Header  200 {integer} X-RateLimit-Remaining "Requests remaining before throttling; bounded by X-RateLimit-Limit"
// @Header  200 {integer} X-RateLimit-Reset     "Unix timestamp (seconds) when X-RateLimit-Remaining will next equal X-RateLimit-Limit"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Header  429 {integer} Retry-After           "Seconds to wait before retrying"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security APIKey[assets:read]
// @Router /api/v1/assets [get]
func (handler *Handler) ListAssets(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetListFailed, "missing organization context", reqID)
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Filters:     []string{"location", "is_active", "type", "q"},
		BoolFilters: []string{"is_active"},
		Sorts:       []string{"identifier", "name", "created_at", "updated_at"},
	})
	if err != nil {
		httputil.RespondListParamError(w, req, err, reqID)
		return
	}

	f := asset.ListFilter{
		LocationIdentifiers: params.Filters["location"],
		Limit:               params.Limit,
		Offset:              params.Offset,
	}
	if vs, ok := params.Filters["is_active"]; ok && len(vs) > 0 {
		b := vs[0] == "true"
		f.IsActive = &b
	}
	if vs, ok := params.Filters["type"]; ok && len(vs) > 0 {
		f.Type = &vs[0]
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
			apierrors.AssetListFailed, err.Error(), reqID)
		return
	}

	total, err := handler.storage.CountAssetsFiltered(req.Context(), orgID, f)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetCountFailed, err.Error(), reqID)
		return
	}

	out := make([]asset.PublicAssetView, 0, len(items))
	for _, a := range items {
		out = append(out, asset.ToPublicAssetView(a))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        out,
		"limit":       params.Limit,
		"offset":      params.Offset,
		"total_count": total,
	})
}

// @Summary Get asset by natural identifier
// @Description Retrieve an asset by its natural identifier. Returns 404 if the asset does not exist.
// @Tags assets,public
// @ID assets.get
// @Param identifier path string true "Asset identifier (natural key)"
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
// @Security APIKey[assets:read]
// @Router /api/v1/assets/{identifier} [get]
func (handler *Handler) GetAssetByIdentifier(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetGetFailed, "missing organization context", reqID)
		return
	}

	identifier := chi.URLParam(req, "identifier")
	if identifier == "" {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Missing identifier", "", reqID)
		return
	}

	a, err := handler.storage.GetAssetByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetGetFailed, err.Error(), reqID)
		return
	}
	if a == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data": asset.ToPublicAssetView(*a),
	})
}

// @Summary      Add an identifier to an asset
// @Description  Attach a tag identifier (RFID EPC, BLE beacon ID, barcode, etc.) to an existing asset.
// @Description  The identifier must be unique within the organization.
// @Tags         assets,public
// @ID           assets.identifiers.add
// @Accept       json
// @Produce      json
// @Param        identifier  path  string                         true  "Asset identifier"
// @Param        request     body  shared.TagIdentifierRequest    true  "Tag identifier to attach"
// @Success      201  {object}  map[string]any                "data: shared.TagIdentifier"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[assets:write]
// @Router       /api/v1/assets/{identifier}/identifiers [post]
func (handler *Handler) AddIdentifier(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetCreateFailed, "missing organization context", requestID)
		return
	}

	identifier := chi.URLParam(r, "identifier")

	existingAsset, err := handler.storage.GetAssetByIdentifier(r.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetGetFailed, err.Error(), requestID)
		return
	}
	if existingAsset == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", requestID)
		return
	}

	handler.doAddAssetIdentifier(w, r, orgID, existingAsset.ID)
}

// doAddAssetIdentifier decodes the identifier body, validates it, and inserts
// via storage. Caller must have already verified that (orgID, assetID) names
// a real asset — storage.AddIdentifierToAsset does NOT cross-check ownership
// before INSERT, so skipping the pre-check would allow cross-org identifier
// attachment.
func (handler *Handler) doAddAssetIdentifier(w http.ResponseWriter, r *http.Request, orgID, assetID int) {
	requestID := middleware.GetRequestID(r.Context())

	var request shared.TagIdentifierRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	tagIdent, err := handler.storage.AddIdentifierToAsset(r.Context(), orgID, assetID, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.AssetCreateFailed, err.Error(), requestID)
			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": tagIdent})
}

// @Summary      Remove an identifier from an asset
// @Description  Detach a tag identifier from an asset by its identifier record ID.
// @Tags         assets,public
// @ID           assets.identifiers.remove
// @Accept       json
// @Produce      json
// @Param        identifier    path  string  true  "Asset identifier"
// @Param        identifierId  path  int     true  "Identifier ID"
// @Success      204  "deleted"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[assets:write]
// @Router       /api/v1/assets/{identifier}/identifiers/{identifierId} [delete]
func (handler *Handler) RemoveIdentifier(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetDeleteFailed, "missing organization context", requestID)
		return
	}

	identifier := chi.URLParam(r, "identifier")

	existingAsset, err := handler.storage.GetAssetByIdentifier(r.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetGetFailed, err.Error(), requestID)
		return
	}
	if existingAsset == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", requestID)
		return
	}

	handler.doRemoveAssetIdentifier(w, r, orgID, existingAsset.ID)
}

// doRemoveAssetIdentifier parses {identifierId} and soft-deletes via storage.
// Storage guards cross-asset / cross-org misuse itself (EXISTS subquery on
// asset_id + org_id), so a missing match surfaces as deleted=false rather
// than an error.
func (handler *Handler) doRemoveAssetIdentifier(w http.ResponseWriter, r *http.Request, orgID, assetID int) {
	requestID := middleware.GetRequestID(r.Context())

	identifierIDParam := chi.URLParam(r, "identifierId")
	identifierID, err := strconv.Atoi(identifierIDParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"invalid identifier ID", err.Error(), requestID)
		return
	}

	_, err = handler.storage.RemoveAssetIdentifier(r.Context(), orgID, assetID, identifierID)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// @Summary Update asset by surrogate ID (internal)
// @Description Session-auth-only variant for the frontend. Public consumers must use {identifier}.
// @Tags assets,internal
// @Param id path int true "Asset surrogate ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/assets/by-id/{id} [put]
func (handler *Handler) UpdateAssetByID(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetUpdateFailed, "missing organization context", reqID)
		return
	}

	id, ok := handler.parseAndVerifyAssetID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doUpdateAsset(w, req, orgID, id)
}

// @Summary Delete asset by surrogate ID (internal)
// @Tags assets,internal
// @Router /api/v1/assets/by-id/{id} [delete]
func (handler *Handler) DeleteAssetByID(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetDeleteFailed, "missing organization context", reqID)
		return
	}

	id, ok := handler.parseAndVerifyAssetID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doDeleteAsset(w, req, orgID, id)
}

// @Summary Add identifier to asset by surrogate ID (internal)
// @Tags assets,internal
// @Router /api/v1/assets/by-id/{id}/identifiers [post]
func (handler *Handler) AddIdentifierByID(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetCreateFailed, "missing organization context", reqID)
		return
	}

	id, ok := handler.parseAndVerifyAssetID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doAddAssetIdentifier(w, req, orgID, id)
}

// @Summary Remove identifier from asset by surrogate ID (internal)
// @Tags assets,internal
// @Router /api/v1/assets/by-id/{id}/identifiers/{identifierId} [delete]
func (handler *Handler) RemoveIdentifierByID(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetDeleteFailed, "missing organization context", reqID)
		return
	}

	id, ok := handler.parseAndVerifyAssetID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doRemoveAssetIdentifier(w, req, orgID, id)
}

// parseAndVerifyAssetID extracts {id}, parses it as a surrogate int, and
// verifies the asset exists and belongs to the caller's org. Writes an
// appropriate 400 / 404 / 500 response and returns ok=false on any failure.
func (handler *Handler) parseAndVerifyAssetID(w http.ResponseWriter, req *http.Request, orgID int, reqID string) (int, bool) {
	idParam := chi.URLParam(req, "id")
	id, err := httputil.ParseSurrogateID(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.AssetGetInvalidID, idParam), err.Error(), reqID)
		return 0, false
	}

	a, err := handler.storage.GetAssetByID(req.Context(), orgID, &id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetGetFailed, err.Error(), reqID)
		return 0, false
	}
	if a == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", reqID)
		return 0, false
	}

	return a.ID, true
}

// RegisterRoutes keeps only session-only surface (bulk CSV). Public write and
// identifier routes are registered directly in internal/cmd/serve/router.go
// under the EitherAuth + WriteAudit + RequireScope group. Public reads are
// also registered there (per TRA-396).
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/assets/bulk", handler.UploadCSV)
	r.Get("/api/v1/assets/bulk/{jobId}", handler.GetJobStatus)
}
