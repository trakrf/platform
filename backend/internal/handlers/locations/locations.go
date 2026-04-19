package locations

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
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

var validate = validator.New()

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
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:write]
// @Router       /api/v1/locations [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationCreateFailed, "missing organization context", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	var request location.CreateLocationWithIdentifiersRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.InvalidJSON, err.Error(), requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.ValidationFailed, err.Error(), requestID)
		return
	}

	var result *location.LocationView
	var err error

	if len(request.Identifiers) > 0 {
		result, err = handler.storage.CreateLocationWithIdentifiers(r.Context(), orgID, request)
	} else {
		result, err = handler.createLocationWithoutIdentifiers(r.Context(), orgID, request)
	}

	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationCreateFailed, err.Error(), requestID)
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
	idParam := chi.URLParam(req, "id")

	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationUpdateInvalidID, idParam), err.Error(), ctx)
		return
	}

	var request location.UpdateLocationRequest

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.LocationUpdateInvalidReq, err.Error(), ctx)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.ValidationFailed, err.Error(), ctx)
		return
	}

	result, err := handler.storage.UpdateLocation(req.Context(), id, request)

	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationUpdateFailed, err.Error(), ctx)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]*location.Location{"data": result})
}

// @Summary      Retrieve a location
// @Description  Fetch a single location by its numeric ID, including associated tag identifiers.
// @Description  Pass include=relations to also receive the location's ancestors and immediate children in the response.
// @Tags         locations,public
// @Accept       json
// @Produce      json
// @Param        id       path   int     true   "Location ID"
// @Param        include  query  string  false  "Pass 'relations' to include ancestors and children"  Enums(relations)
// @Success      202  {object}  map[string]any                "data: location.LocationView"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:read]
// @Router       /api/v1/locations/{id} [get]
func (handler *Handler) Get(w http.ResponseWriter, req *http.Request) {
	idParam := chi.URLParam(req, "id")
	ctx := middleware.GetRequestID(req.Context())

	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationGetInvalidID, idParam), err.Error(), ctx)
		return
	}

	includeParam := req.URL.Query().Get("include")
	includeRelations := includeParam == "relations"

	result, err := handler.storage.GetLocationViewByID(req.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}

	if includeRelations {
		locWithRelations, relErr := handler.storage.GetLocationWithRelations(req.Context(), id)
		if relErr == nil && locWithRelations != nil {
			result.Ancestors = locWithRelations.Ancestors
			result.Children = locWithRelations.Children
		}
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]*location.LocationView{"data": result})
}

// @Summary      Delete a location
// @Description  Soft-delete a location by its numeric ID. The location is marked inactive and removed from future list results.
// @Tags         locations,public
// @Accept       json
// @Produce      json
// @Param        id  path  int  true  "Location ID"
// @Success      202  {object}  map[string]bool               "deleted: true/false"
// @Failure      401  {object}  modelerrors.ErrorResponse     "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse     "forbidden"
// @Failure      404  {object}  modelerrors.ErrorResponse     "not_found"
// @Failure      429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse     "internal_error"
// @Security     APIKey[locations:write]
// @Router       /api/v1/locations/{id} [delete]
func (handler *Handler) Delete(w http.ResponseWriter, req *http.Request) {
	idParam := chi.URLParam(req, "id")
	ctx := middleware.GetRequestID(req.Context())

	id, err := strconv.Atoi(idParam)

	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationDeleteInvalidID, idParam), err.Error(), ctx)
		return
	}

	deleted, err := handler.storage.DeleteLocation(req.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationDeleteFailed, err.Error(), ctx)
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

// @Summary      List locations
// @Description  Return a paginated list of all locations in the organization, each with their associated tag identifiers.
// @Tags         locations,public
// @Accept       json
// @Produce      json
// @Param        limit   query  int  false  "Max results (default 10, max 200)"  default(10)  minimum(1)  maximum(200)
// @Param        offset  query  int  false  "Pagination offset"                   default(0)   minimum(0)
// @Success      202  {object}  ListLocationsResponse             "Paginated list of locations with metadata"
// @Failure      400  {object}  modelerrors.ErrorResponse         "bad_request"
// @Failure      401  {object}  modelerrors.ErrorResponse         "unauthorized"
// @Failure      403  {object}  modelerrors.ErrorResponse         "forbidden"
// @Failure      429  {object}  modelerrors.ErrorResponse         "rate_limited"
// @Failure      500  {object}  modelerrors.ErrorResponse         "internal_error"
// @Security     APIKey[locations:read]
// @Router       /api/v1/locations [get]
func (handler *Handler) List(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	claims := middleware.GetUserClaims(req)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationListFailed, "missing organization context", ctx)
		return
	}
	orgID := *claims.CurrentOrgID

	limit := 10
	offset := 0

	if limitStr := req.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	if offsetStr := req.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	locations, err := handler.storage.ListLocationViews(req.Context(), orgID, limit, offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationListFailed, err.Error(), ctx)
		return
	}

	totalCount, err := handler.storage.CountAllLocations(req.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationCountFailed, err.Error(), ctx)
		return
	}

	response := map[string]any{
		"data":        locations,
		"count":       len(locations),
		"offset":      offset,
		"total_count": totalCount,
	}

	httputil.WriteJSON(w, http.StatusAccepted, response)
}

// @Summary      List location ancestors
// @Description  Return all ancestor locations from the root of the hierarchy down to the immediate parent of the specified location.
// @Tags         locations,public
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
// @Tags         locations,public
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
// @Tags         locations,public
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

	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationCreateFailed, "missing organization context", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

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
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.InvalidJSON, err.Error(), requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.ValidationFailed, err.Error(), requestID)
		return
	}

	identifier, err := handler.storage.AddIdentifierToLocation(r.Context(), orgID, locationID, request)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationCreateFailed, err.Error(), requestID)
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

	idParam := chi.URLParam(r, "id")
	_, err := strconv.Atoi(idParam)
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

	deleted, err := handler.storage.RemoveIdentifier(r.Context(), identifierID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationDeleteFailed, err.Error(), requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]bool{"deleted": deleted})
}

func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/locations", handler.List)
	r.Get("/api/v1/locations/{id}", handler.Get)
	r.Post("/api/v1/locations", handler.Create)
	r.Put("/api/v1/locations/{id}", handler.Update)
	r.Delete("/api/v1/locations/{id}", handler.Delete)
	r.Post("/api/v1/locations/{id}/identifiers", handler.AddIdentifier)
	r.Delete("/api/v1/locations/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
	r.Get("/api/v1/locations/{id}/ancestors", handler.GetAncestors)
	r.Get("/api/v1/locations/{id}/descendants", handler.GetDescendants)
	r.Get("/api/v1/locations/{id}/children", handler.GetChildren)
}
