package locations

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

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

// @Summary      Create a location
// @Description  Create a new location in the hierarchy, optionally with one or more tag identifiers.
// @Description  Set ParentLocationID to nest the location under an existing parent. The Location response header contains the canonical URL.
// @Tags         locations,public
// @ID           locations.create
// @Accept       json
// @Produce      json
// @Param        request  body  location.CreateLocationWithIdentifiersRequest  true  "Location to create with optional identifiers"
// @Success      201  {object}  locations.CreateLocationResponse
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:write]
// @Router       /api/v1/locations [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationCreateFailed, "missing organization context", requestID)
		return
	}

	var request location.CreateLocationWithIdentifiersRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	result, err := handler.storage.CreateLocationWithIdentifiers(r.Context(), orgID, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.LocationCreateFailed, err.Error(), requestID)
			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.Header().Set("Location", "/api/v1/locations/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": location.ToPublicLocationView(*result)})
}

// @Summary      Update a location
// @Description  Update mutable fields on an existing location. Only fields included in the request body are changed.
// @Tags         locations,public
// @ID           locations.update
// @Accept       json
// @Produce      json
// @Param        identifier  path  string                           true  "Location identifier"
// @Param        request     body  location.UpdateLocationRequest   true  "Fields to update"
// @Success      200  {object}  locations.UpdateLocationResponse
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:write]
// @Router       /api/v1/locations/{identifier} [put]
func (handler *Handler) Update(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationUpdateFailed, "missing organization context", ctx)
		return
	}

	identifier := chi.URLParam(req, "identifier")

	loc, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}
	if loc == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}

	handler.doUpdate(w, req, orgID, loc.ID)
}

// doUpdate decodes, validates, and stores the location update. Caller must
// have already verified that (orgID, id) names a real location.
func (handler *Handler) doUpdate(w http.ResponseWriter, req *http.Request, orgID, id int) {
	reqID := middleware.GetRequestID(req.Context())

	var request location.UpdateLocationRequest
	if err := httputil.DecodeJSON(req, &request); err != nil {
		httputil.RespondDecodeError(w, req, err, reqID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, req, err, reqID)
		return
	}

	result, err := handler.storage.UpdateLocation(req.Context(), orgID, id, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.LocationUpdateFailed, err.Error(), reqID)
			return
		}
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": location.ToPublicLocationView(*result)})
}

// @Summary Delete location
// @Description Soft-delete a location by its natural identifier. The location is marked inactive and removed from future list results.
// @Tags locations,public
// @ID locations.delete
// @Accept json
// @Produce json
// @Param identifier path string true "Location identifier"
// @Success 204 "deleted"
// @Failure 400 {object} modelerrors.ErrorResponse "bad_request"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "forbidden"
// @Failure 404 {object} modelerrors.ErrorResponse "not_found"
// @Failure 429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure 500 {object} modelerrors.ErrorResponse "internal_error"
// @Security APIKey[locations:write]
// @Router /api/v1/locations/{identifier} [delete]
func (handler *Handler) Delete(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationDeleteFailed, "missing organization context", ctx)
		return
	}

	identifier := chi.URLParam(req, "identifier")

	loc, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}
	if loc == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}

	handler.doDelete(w, req, orgID, loc.ID)
}

// doDelete soft-deletes the location via storage. Caller must have already
// verified that (orgID, id) names a real location.
func (handler *Handler) doDelete(w http.ResponseWriter, req *http.Request, orgID, id int) {
	reqID := middleware.GetRequestID(req.Context())

	deleted, err := handler.storage.DeleteLocation(req.Context(), orgID, id)
	if err != nil {
		httputil.RespondStorageError(w, req, err, reqID)
		return
	}

	if !deleted {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", reqID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListLocationsResponse is the typed envelope returned by GET /api/v1/locations.
type ListLocationsResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}

// GetLocationResponse is the typed envelope returned by GET /api/v1/locations/{identifier}.
type GetLocationResponse struct {
	Data location.PublicLocationView `json:"data"`
}

// CreateLocationResponse is the typed envelope returned by POST /api/v1/locations.
type CreateLocationResponse struct {
	Data location.PublicLocationView `json:"data"`
}

// UpdateLocationResponse is the typed envelope returned by PUT /api/v1/locations/{identifier}
// and PUT /api/v1/locations/by-id/{id}.
type UpdateLocationResponse struct {
	Data location.PublicLocationView `json:"data"`
}

// @Summary List locations
// @Tags locations,public
// @ID locations.list
// @Param limit    query int    false "max 200"
// @Param offset   query int    false "pagination offset"
// @Param parent   query string false "filter by parent identifier (may repeat)"
// @Param is_active query bool  false "filter by active flag"
// @Param q        query string false "fuzzy search on name, identifier, description"
// @Param sort     query string false "comma-separated, prefix '-' for DESC"
// @Success 200 {object} locations.ListLocationsResponse
// @Header  200 {integer} X-RateLimit-Limit     "Steady-state requests/min for this API key"
// @Header  200 {integer} X-RateLimit-Remaining "Tokens left in bucket at response time"
// @Header  200 {integer} X-RateLimit-Reset     "Unix timestamp when bucket fully refills"
// @Failure 429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Header  429 {integer} Retry-After           "Seconds to wait before retrying"
// @Security APIKey[locations:read]
// @Router /api/v1/locations [get]
func (handler *Handler) ListLocations(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationListFailed, "missing organization context", reqID)
		return
	}

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{
		Filters:     []string{"parent", "is_active", "q"},
		BoolFilters: []string{"is_active"},
		Sorts:       []string{"path", "identifier", "name", "created_at"},
	})
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid list parameters", err.Error(), reqID)
		return
	}

	f := location.ListFilter{
		ParentIdentifiers: params.Filters["parent"],
		Limit:             params.Limit,
		Offset:            params.Offset,
	}
	if vs, ok := params.Filters["is_active"]; ok && len(vs) > 0 {
		b := vs[0] == "true"
		f.IsActive = &b
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
			apierrors.LocationListFailed, err.Error(), reqID)
		return
	}

	total, err := handler.storage.CountLocationsFiltered(req.Context(), orgID, f)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationListFailed, err.Error(), reqID)
		return
	}

	out := make([]location.PublicLocationView, 0, len(items))
	for _, l := range items {
		out = append(out, location.ToPublicLocationView(l))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        out,
		"limit":       params.Limit,
		"offset":      params.Offset,
		"total_count": total,
	})
}

// @Summary Get location by natural identifier
// @Tags locations,public
// @ID locations.get
// @Param identifier path string true "Location identifier (natural key)"
// @Success 200 {object} locations.GetLocationResponse
// @Header  200 {integer} X-RateLimit-Limit     "Steady-state requests/min for this API key"
// @Header  200 {integer} X-RateLimit-Remaining "Tokens left in bucket at response time"
// @Header  200 {integer} X-RateLimit-Reset     "Unix timestamp when bucket fully refills"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Header  429 {integer} Retry-After           "Seconds to wait before retrying"
// @Security APIKey[locations:read]
// @Router /api/v1/locations/{identifier} [get]
func (handler *Handler) GetLocationByIdentifier(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationGetFailed, "missing organization context", reqID)
		return
	}

	identifier := chi.URLParam(req, "identifier")
	if identifier == "" {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Missing identifier", "", reqID)
		return
	}

	l, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), reqID)
		return
	}
	if l == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", reqID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data": location.ToPublicLocationView(*l),
	})
}

// @Summary Get location by surrogate ID (internal)
// @Tags locations,internal
// @Router /api/v1/locations/by-id/{id} [get]
func (handler *Handler) GetLocationByID(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationGetFailed, "missing organization context", reqID)
		return
	}

	idParam := chi.URLParam(req, "id")
	id, err := httputil.ParseSurrogateID(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid location ID", err.Error(), reqID)
		return
	}

	view, err := handler.storage.GetLocationViewByID(req.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), reqID)
		return
	}
	if view == nil || view.OrgID != orgID {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", reqID)
		return
	}

	public := location.ToPublicLocationView(location.LocationWithParent{
		LocationView: *view,
	})

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": public})
}

// @Summary      List location ancestors
// @Description  Return all ancestor locations from the root of the hierarchy down to the immediate parent of the specified location.
// @Tags         locations,public
// @ID           locations.ancestors
// @Accept       json
// @Produce      json
// @Param        identifier  path  string  true  "Location identifier"
// @Success      200  {object}  map[string]any                "data: []location.PublicLocationView"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:read]
// @Router       /api/v1/locations/{identifier}/ancestors [get]
func (handler *Handler) GetAncestors(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())
	identifier := chi.URLParam(req, "identifier")

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationGetFailed, "missing organization context", ctx)
		return
	}

	loc, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}
	if loc == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}
	id := loc.ID

	results, err := handler.storage.GetAncestors(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": toPublicLocationViews(results)})
}

// @Summary      List location descendants
// @Description  Return all descendant locations (children, grandchildren, etc.) beneath the specified location in the hierarchy.
// @Tags         locations,public
// @ID           locations.descendants
// @Accept       json
// @Produce      json
// @Param        identifier  path  string  true  "Location identifier"
// @Success      200  {object}  map[string]any                "data: []location.PublicLocationView"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:read]
// @Router       /api/v1/locations/{identifier}/descendants [get]
func (handler *Handler) GetDescendants(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())
	identifier := chi.URLParam(req, "identifier")

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationGetFailed, "missing organization context", ctx)
		return
	}

	loc, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}
	if loc == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}
	id := loc.ID

	results, err := handler.storage.GetDescendants(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": toPublicLocationViews(results)})
}

// @Summary      List location children
// @Description  Return the immediate child locations of the specified location (one level deep only).
// @Tags         locations,public
// @ID           locations.children
// @Accept       json
// @Produce      json
// @Param        identifier  path  string  true  "Location identifier"
// @Success      200  {object}  map[string]any                "data: []location.PublicLocationView"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:read]
// @Router       /api/v1/locations/{identifier}/children [get]
func (handler *Handler) GetChildren(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())
	identifier := chi.URLParam(req, "identifier")

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationGetFailed, "missing organization context", ctx)
		return
	}

	loc, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}
	if loc == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}
	id := loc.ID

	results, err := handler.storage.GetChildren(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": toPublicLocationViews(results)})
}

// toPublicLocationViews is the hierarchy-endpoint response adapter. Each input carries
// the parent's natural key (LEFT JOIN) and its tag identifiers (bulk-fetched) so the
// output shape is identical to GET /locations/{identifier}.
func toPublicLocationViews(locs []location.LocationWithParent) []location.PublicLocationView {
	views := make([]location.PublicLocationView, len(locs))
	for i, l := range locs {
		views[i] = location.ToPublicLocationView(l)
	}
	return views
}

// @Summary      Add an identifier to a location
// @Description  Attach a tag identifier (RFID EPC, BLE beacon ID, barcode, etc.) to an existing location.
// @Description  The identifier must be unique within the organization.
// @Tags         locations,public
// @ID           locations.identifiers.add
// @Accept       json
// @Produce      json
// @Param        identifier  path  string                         true  "Location identifier"
// @Param        request     body  shared.TagIdentifierRequest    true  "Tag identifier to attach"
// @Success      201  {object}  map[string]any                "data: shared.TagIdentifier"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:write]
// @Router       /api/v1/locations/{identifier}/identifiers [post]
func (handler *Handler) AddIdentifier(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationCreateFailed, "missing organization context", requestID)
		return
	}

	identifier := chi.URLParam(r, "identifier")

	loc, err := handler.storage.GetLocationByIdentifier(r.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), requestID)
		return
	}
	if loc == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", requestID)
		return
	}

	handler.doAddLocationIdentifier(w, r, orgID, loc.ID)
}

// doAddLocationIdentifier decodes + validates the identifier body and inserts
// via storage. Caller must have already verified that (orgID, locationID)
// names a real location — storage.AddIdentifierToLocation does NOT cross-check
// ownership before INSERT, so skipping the pre-check would allow cross-org
// identifier attachment.
func (handler *Handler) doAddLocationIdentifier(w http.ResponseWriter, r *http.Request, orgID, locationID int) {
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

	tagIdent, err := handler.storage.AddIdentifierToLocation(r.Context(), orgID, locationID, request)
	if err != nil {
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.LocationCreateFailed, err.Error(), requestID)
			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": tagIdent})
}

// @Summary      Remove an identifier from a location
// @Description  Detach a tag identifier from a location by its identifier record ID.
// @Tags         locations,public
// @ID           locations.identifiers.remove
// @Accept       json
// @Produce      json
// @Param        identifier    path  string  true  "Location identifier"
// @Param        identifierId  path  int     true  "Identifier ID"
// @Success      204  "deleted"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:write]
// @Router       /api/v1/locations/{identifier}/identifiers/{identifierId} [delete]
func (handler *Handler) RemoveIdentifier(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationDeleteFailed, "missing organization context", requestID)
		return
	}

	identifier := chi.URLParam(r, "identifier")

	loc, err := handler.storage.GetLocationByIdentifier(r.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), requestID)
		return
	}
	if loc == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", requestID)
		return
	}

	handler.doRemoveLocationIdentifier(w, r, orgID, loc.ID)
}

// doRemoveLocationIdentifier parses {identifierId} and soft-deletes via
// storage. Storage guards cross-location / cross-org misuse itself (EXISTS
// subquery on location_id + org_id), so a missing match surfaces as
// deleted=false rather than an error.
func (handler *Handler) doRemoveLocationIdentifier(w http.ResponseWriter, r *http.Request, orgID, locationID int) {
	requestID := middleware.GetRequestID(r.Context())

	identifierIDParam := chi.URLParam(r, "identifierId")
	identifierID, err := strconv.Atoi(identifierIDParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"invalid identifier ID", err.Error(), requestID)
		return
	}

	_, err = handler.storage.RemoveLocationIdentifier(r.Context(), orgID, locationID, identifierID)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// @Summary Update location by surrogate ID (internal)
// @Description Session-auth-only variant for the frontend. Public consumers must use {identifier}.
// @Tags locations,internal
// @Param id path int true "Location surrogate ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/locations/by-id/{id} [put]
func (handler *Handler) UpdateByID(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationUpdateFailed, "missing organization context", reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doUpdate(w, req, orgID, id)
}

// @Summary Delete location by surrogate ID (internal)
// @Tags locations,internal
// @Router /api/v1/locations/by-id/{id} [delete]
func (handler *Handler) DeleteByID(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationDeleteFailed, "missing organization context", reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doDelete(w, req, orgID, id)
}

// @Summary Add identifier to location by surrogate ID (internal)
// @Tags locations,internal
// @Router /api/v1/locations/by-id/{id}/identifiers [post]
func (handler *Handler) AddIdentifierByID(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationCreateFailed, "missing organization context", reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doAddLocationIdentifier(w, req, orgID, id)
}

// @Summary Remove identifier from location by surrogate ID (internal)
// @Tags locations,internal
// @Router /api/v1/locations/by-id/{id}/identifiers/{identifierId} [delete]
func (handler *Handler) RemoveIdentifierByID(w http.ResponseWriter, req *http.Request) {
	reqID := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationDeleteFailed, "missing organization context", reqID)
		return
	}

	id, ok := handler.parseAndVerifyLocationID(w, req, orgID, reqID)
	if !ok {
		return
	}

	handler.doRemoveLocationIdentifier(w, req, orgID, id)
}

// parseAndVerifyLocationID extracts {id}, parses it as a surrogate int, and
// verifies the location exists and belongs to the caller's org. Writes an
// appropriate 400 / 404 / 500 response and returns ok=false on any failure.
func (handler *Handler) parseAndVerifyLocationID(w http.ResponseWriter, req *http.Request, orgID int, reqID string) (int, bool) {
	idParam := chi.URLParam(req, "id")
	id, err := httputil.ParseSurrogateID(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationGetInvalidID, idParam), err.Error(), reqID)
		return 0, false
	}

	loc, err := handler.storage.GetLocationByID(req.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), reqID)
		return 0, false
	}
	if loc == nil || loc.OrgID != orgID {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", reqID)
		return 0, false
	}

	return loc.ID, true
}

// RegisterRoutes keeps only session-only surface (hierarchy by-identifier). Public write
// routes are registered in internal/cmd/serve/router.go under EitherAuth +
// WriteAudit + RequireScope. Public reads likewise (per TRA-396).
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/locations/{identifier}/ancestors", handler.GetAncestors)
	r.Get("/api/v1/locations/{identifier}/descendants", handler.GetDescendants)
	r.Get("/api/v1/locations/{identifier}/children", handler.GetChildren)
}
