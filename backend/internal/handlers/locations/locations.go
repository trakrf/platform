package locations

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/location"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
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

// @Summary Create location
// @Description Create a new location in the hierarchy
// @Tags locations
// @Accept json
// @Produce json
// @Param request body location.CreateLocationRequest true "Location to create"
// @Success 201 {object} map[string]any "data: location.Location"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid JSON or validation error"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/locations [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationCreateFailed, "missing organization context", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	var request location.CreateLocationRequest
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

	loc := location.Location{
		OrgID:              orgID,
		Name:               request.Name,
		Identifier:         request.Identifier,
		ParentLocationID:   request.ParentLocationID,
		Description:        request.Description,
		ValidFrom:          request.ValidFrom,
		ValidTo:            request.ValidTo,
		IsActive:           request.IsActive,
	}

	result, err := handler.storage.CreateLocation(r.Context(), loc)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationCreateFailed, err.Error(), requestID)
		return
	}

	w.Header().Set("Location", "/api/v1/locations/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// @Summary Update location
// @Description Update an existing location by ID
// @Tags locations
// @Accept json
// @Produce json
// @Param id path int true "Location ID"
// @Param request body location.UpdateLocationRequest true "Location update data"
// @Success 202 {object} map[string]any "data: location.Location"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid ID, JSON, or validation error"
// @Failure 404 {object} modelerrors.ErrorResponse "Location not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/locations/{id} [put]
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

// @Summary Get location
// @Description Get a location by ID, optionally including children and ancestors
// @Tags locations
// @Accept json
// @Produce json
// @Param id path int true "Location ID"
// @Param include query string false "Include relations: 'relations' for children+ancestors"
// @Success 202 {object} map[string]any "data: location.Location"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid location ID"
// @Failure 404 {object} modelerrors.ErrorResponse "Location not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/locations/{id} [get]
func (handler *Handler) Get(w http.ResponseWriter, req *http.Request) {
	idParam := chi.URLParam(req, "id")
	ctx := middleware.GetRequestID(req.Context())

	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.LocationGetInvalidID, idParam), err.Error(), ctx)
		return
	}

	// Check if client wants children and ancestors included
	includeParam := req.URL.Query().Get("include")
	includeRelations := includeParam == "relations"

	var result *location.Location
	if includeRelations {
		// Use ltree-powered single query to fetch location + children + ancestors
		result, err = handler.storage.GetLocationWithRelations(req.Context(), id)
	} else {
		// Basic fetch without relations
		result, err = handler.storage.GetLocationByID(req.Context(), id)
	}

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

	httputil.WriteJSON(w, http.StatusAccepted, map[string]*location.Location{"data": result})
}

// @Summary Delete location
// @Description Soft delete a location by ID
// @Tags locations
// @Accept json
// @Produce json
// @Param id path int true "Location ID"
// @Success 202 {object} map[string]bool "deleted: true/false"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid location ID"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/locations/{id} [delete]
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
	Data       []location.Location `json:"data"`
	Count      int                 `json:"count" example:"10"`
	Offset     int                 `json:"offset" example:"0"`
	TotalCount int                 `json:"total_count" example:"100"`
}

// @Summary List locations
// @Description Get a paginated list of all locations with pagination metadata
// @Tags locations
// @Accept json
// @Produce json
// @Param limit query int false "Number of locations to return (default: 10)" minimum(1) default(10)
// @Param offset query int false "Number of locations to skip for pagination (default: 0)" minimum(0) default(0)
// @Success 202 {object} ListLocationsResponse "Paginated list of locations with metadata"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/locations [get]
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

	locations, err := handler.storage.ListAllLocations(req.Context(), orgID, limit, offset)
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

// @Summary Get location ancestors
// @Description Get all ancestor locations from root to parent
// @Tags locations
// @Accept json
// @Produce json
// @Param id path int true "Location ID"
// @Success 202 {object} map[string]any "data: []location.Location"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid location ID"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/locations/{id}/ancestors [get]
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

// @Summary Get location descendants
// @Description Get all descendant locations (children at all levels)
// @Tags locations
// @Accept json
// @Produce json
// @Param id path int true "Location ID"
// @Success 202 {object} map[string]any "data: []location.Location"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid location ID"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/locations/{id}/descendants [get]
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

// @Summary Get location children
// @Description Get immediate children of a location
// @Tags locations
// @Accept json
// @Produce json
// @Param id path int true "Location ID"
// @Success 202 {object} map[string]any "data: []location.Location"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid location ID"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/locations/{id}/children [get]
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

func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/locations", handler.List)
	r.Get("/api/v1/locations/{id}", handler.Get)
	r.Post("/api/v1/locations", handler.Create)
	r.Put("/api/v1/locations/{id}", handler.Update)
	r.Delete("/api/v1/locations/{id}", handler.Delete)
	r.Get("/api/v1/locations/{id}/ancestors", handler.GetAncestors)
	r.Get("/api/v1/locations/{id}/descendants", handler.GetDescendants)
	r.Get("/api/v1/locations/{id}/children", handler.GetChildren)
}
