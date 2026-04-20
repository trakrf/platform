package locations

import (
	"context"
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

func (handler *Handler) createLocationWithoutIdentifiers(ctx context.Context, orgID int, request location.CreateLocationWithIdentifiersRequest) (*location.LocationView, error) {
	var validTo *time.Time
	if request.ValidTo != nil && !request.ValidTo.IsZero() {
		t := request.ValidTo.ToTime()
		validTo = &t
	}

	loc := location.Location{
		OrgID:            orgID,
		Name:             request.Name,
		Identifier:       request.Identifier,
		ParentLocationID: request.ParentLocationID,
		Description:      request.Description,
		ValidFrom:        request.ValidFrom.ToTime(),
		ValidTo:          validTo,
		IsActive:         request.IsActive,
	}

	baseLoc, err := handler.storage.CreateLocation(ctx, loc)
	if err != nil {
		return nil, err
	}

	return &location.LocationView{Location: *baseLoc, Identifiers: []shared.TagIdentifier{}}, nil
}

// @Summary      Create a location
// @Description  Create a new location in the hierarchy, optionally with one or more tag identifiers.
// @Description  Set ParentLocationID to nest the location under an existing parent. The Location response header contains the canonical URL.
// @Tags         locations,public
// @Accept       json
// @Produce      json
// @Param        request  body  location.CreateLocationWithIdentifiersRequest  true  "Location to create with optional identifiers"
// @Success      201  {object}  map[string]any                "data: location.LocationView"
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

	var result *location.LocationView

	if len(request.Identifiers) > 0 {
		result, err = handler.storage.CreateLocationWithIdentifiers(r.Context(), orgID, request)
	} else {
		result, err = handler.createLocationWithoutIdentifiers(r.Context(), orgID, request)
	}

	if err != nil {
		// Storage returns "already exists" / "already exist" strings for unique violations
		// (SQLSTATE 23505 is unwrapped to a plain string by the storage layer).
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.LocationCreateFailed, err.Error(), requestID)
			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	w.Header().Set("Location", "/api/v1/locations/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// @Summary      Update a location
// @Description  Update mutable fields on an existing location. Only fields included in the request body are changed.
// @Tags         locations,public
// @Accept       json
// @Produce      json
// @Param        id       path  int                              true  "Location ID"
// @Param        request  body  location.UpdateLocationRequest   true  "Fields to update"
// @Success      202  {object}  map[string]any                "data: location.Location"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      409  {object}  modelerrors.ErrorResponse     "conflict"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:write]
// @Router       /api/v1/locations/{id} [put]
func (handler *Handler) Update(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationUpdateFailed, "missing organization context", ctx)
		return
	}

	idParam := chi.URLParam(req, "id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationUpdateInvalidID, idParam), err.Error(), ctx)
		return
	}

	var request location.UpdateLocationRequest
	if err := httputil.DecodeJSON(req, &request); err != nil {
		httputil.RespondDecodeError(w, req, err, ctx)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, req, err, ctx)
		return
	}

	result, err := handler.storage.UpdateLocation(req.Context(), orgID, id, request)

	if err != nil {
		// Storage returns "already exists" / "already exist" strings for unique violations
		// (SQLSTATE 23505 is unwrapped to a plain string by the storage layer).
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.LocationUpdateFailed, err.Error(), ctx)
			return
		}
		httputil.RespondStorageError(w, req, err, ctx)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]*location.Location{"data": result})
}

// @Summary Delete location
// @Description Soft delete a location by ID
// @Tags locations,public
// @Accept json
// @Produce json
// @Param id path int true "Location ID"
// @Success 202 {object} map[string]bool "deleted: true/false"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid location ID"
// @Failure 401 {object} modelerrors.ErrorResponse "unauthorized"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security APIKey[locations:write]
// @Router /api/v1/locations/{id} [delete]
func (handler *Handler) Delete(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationDeleteFailed, "missing organization context", ctx)
		return
	}

	idParam := chi.URLParam(req, "id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationDeleteInvalidID, idParam), err.Error(), ctx)
		return
	}

	deleted, err := handler.storage.DeleteLocation(req.Context(), orgID, id)
	if err != nil {
		httputil.RespondStorageError(w, req, err, ctx)
		return
	}

	if !deleted {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]bool{"deleted": deleted})
}

type ListLocationsResponse struct {
	Data       []location.LocationView `json:"data"`
	Count      int                     `json:"count" example:"10"`
	Offset     int                     `json:"offset" example:"0"`
	TotalCount int                     `json:"total_count" example:"100"`
}

// @Summary List locations
// @Tags locations,public
// @Param limit    query int    false "max 200"
// @Param offset   query int    false "pagination offset"
// @Param parent   query string false "filter by parent identifier (may repeat)"
// @Param is_active query bool  false "filter by active flag"
// @Param q        query string false "fuzzy search on name, identifier, description"
// @Param sort     query string false "comma-separated, prefix '-' for DESC"
// @Success 200 {object} map[string]any
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
		Filters: []string{"parent", "is_active", "q"},
		Sorts:   []string{"path", "identifier", "name", "created_at"},
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
// @Param identifier path string true "Location identifier (natural key)"
// @Success 200 {object} map[string]any
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
// @Tags         locations,internal
// @Accept       json
// @Produce      json
// @Param        id  path  int  true  "Location ID"
// @Success      202  {object}  map[string]any                "data: []location.Location"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:read]
// @Router       /api/v1/locations/{id}/ancestors [get]
func (handler *Handler) GetAncestors(w http.ResponseWriter, req *http.Request) {
	idParam := chi.URLParam(req, "id")
	ctx := middleware.GetRequestID(req.Context())

	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationGetInvalidID, idParam), err.Error(), ctx)
		return
	}

	results, err := handler.storage.GetAncestors(req.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string][]location.Location{"data": results})
}

// @Summary      List location descendants
// @Description  Return all descendant locations (children, grandchildren, etc.) beneath the specified location in the hierarchy.
// @Tags         locations,internal
// @Accept       json
// @Produce      json
// @Param        id  path  int  true  "Location ID"
// @Success      202  {object}  map[string]any                "data: []location.Location"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:read]
// @Router       /api/v1/locations/{id}/descendants [get]
func (handler *Handler) GetDescendants(w http.ResponseWriter, req *http.Request) {
	idParam := chi.URLParam(req, "id")
	ctx := middleware.GetRequestID(req.Context())

	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationGetInvalidID, idParam), err.Error(), ctx)
		return
	}

	results, err := handler.storage.GetDescendants(req.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string][]location.Location{"data": results})
}

// @Summary      List location children
// @Description  Return the immediate child locations of the specified location (one level deep only).
// @Tags         locations,internal
// @Accept       json
// @Produce      json
// @Param        id  path  int  true  "Location ID"
// @Success      202  {object}  map[string]any                "data: []location.Location"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:read]
// @Router       /api/v1/locations/{id}/children [get]
func (handler *Handler) GetChildren(w http.ResponseWriter, req *http.Request) {
	idParam := chi.URLParam(req, "id")
	ctx := middleware.GetRequestID(req.Context())

	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationGetInvalidID, idParam), err.Error(), ctx)
		return
	}

	results, err := handler.storage.GetChildren(req.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string][]location.Location{"data": results})
}

// @Summary      Add an identifier to a location
// @Description  Attach a tag identifier (RFID EPC, BLE beacon ID, barcode, etc.) to an existing location.
// @Description  The identifier must be unique within the organization.
// @Tags         locations,public
// @Accept       json
// @Produce      json
// @Param        id       path  int                            true  "Location ID"
// @Param        request  body  shared.TagIdentifierRequest    true  "Tag identifier to attach"
// @Success      201  {object}  map[string]any                "data: shared.TagIdentifier"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:write]
// @Router       /api/v1/locations/{id}/identifiers [post]
func (handler *Handler) AddIdentifier(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationCreateFailed, "missing organization context", requestID)
		return
	}

	idParam := chi.URLParam(r, "id")
	locationID, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationGetInvalidID, idParam), err.Error(), requestID)
		return
	}

	existingLoc, err := handler.storage.GetLocationByID(r.Context(), locationID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), requestID)
		return
	}
	if existingLoc == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", requestID)
		return
	}

	var request shared.TagIdentifierRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	identifier, err := handler.storage.AddIdentifierToLocation(r.Context(), orgID, locationID, request)
	if err != nil {
		// Storage returns "already exists" / "already exist" strings for unique violations
		// (SQLSTATE 23505 is unwrapped to a plain string by the storage layer).
		if strings.Contains(err.Error(), "already exist") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.LocationCreateFailed, err.Error(), requestID)
			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": identifier})
}

// @Summary      Remove an identifier from a location
// @Description  Detach a tag identifier from a location by its identifier record ID.
// @Tags         locations,public
// @Accept       json
// @Produce      json
// @Param        id            path  int  true  "Location ID"
// @Param        identifierId  path  int  true  "Identifier ID"
// @Success      202  {object}  map[string]bool               "deleted: true/false"
// @Failure      400  {object}  modelerrors.ErrorResponse     "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:write]
// @Router       /api/v1/locations/{id}/identifiers/{identifierId} [delete]
func (handler *Handler) RemoveIdentifier(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationDeleteFailed, "missing organization context", requestID)
		return
	}

	idParam := chi.URLParam(r, "id")
	locationID, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationGetInvalidID, idParam), err.Error(), requestID)
		return
	}

	identifierIDParam := chi.URLParam(r, "identifierId")
	identifierID, err := strconv.Atoi(identifierIDParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"invalid identifier ID", err.Error(), requestID)
		return
	}

	deleted, err := handler.storage.RemoveLocationIdentifier(r.Context(), orgID, locationID, identifierID)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]bool{"deleted": deleted})
}

// RegisterRoutes keeps only session-only surface (hierarchy by-id). Public write
// routes are registered in internal/cmd/serve/router.go under EitherAuth +
// WriteAudit + RequireScope. Public reads likewise (per TRA-396).
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/locations/{id}/ancestors", handler.GetAncestors)
	r.Get("/api/v1/locations/{id}/descendants", handler.GetDescendants)
	r.Get("/api/v1/locations/{id}/children", handler.GetChildren)
}
