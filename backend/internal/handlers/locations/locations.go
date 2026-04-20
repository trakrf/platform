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

// @Summary Create location
// @Description Create a new location in the hierarchy, optionally with tag identifiers
// @Tags locations
// @Accept json
// @Produce json
// @Param request body location.CreateLocationWithIdentifiersRequest true "Location to create with optional identifiers"
// @Success 201 {object} map[string]any "data: location.LocationView"
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

// @Summary List locations
// @Tags locations
// @Param limit    query int    false "max 200"
// @Param offset   query int    false
// @Param parent   query string false "filter by parent identifier (may repeat)"
// @Param is_active query bool  false
// @Param q        query string false
// @Param sort     query string false
// @Success 200 {object} map[string]any
// @Security BearerAuth
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

// GetLocationByIdentifier serves /api/v1/locations/{identifier}.
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
	id, err := strconv.Atoi(idParam)
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

// @Summary Add identifier to location
// @Description Add a tag identifier (RFID, BLE, barcode) to an existing location
// @Tags locations
// @Accept json
// @Produce json
// @Param id path int true "Location ID"
// @Param request body shared.TagIdentifierRequest true "Tag identifier to add"
// @Success 201 {object} map[string]any "data: shared.TagIdentifier"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid request"
// @Failure 404 {object} modelerrors.ErrorResponse "Location not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/locations/{id}/identifiers [post]
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

// @Summary Remove identifier from location
// @Description Remove a tag identifier from a location
// @Tags locations
// @Accept json
// @Produce json
// @Param id path int true "Location ID"
// @Param identifierId path int true "Identifier ID"
// @Success 202 {object} map[string]bool "deleted: true/false"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid request"
// @Failure 404 {object} modelerrors.ErrorResponse "Location or identifier not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/locations/{id}/identifiers/{identifierId} [delete]
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
	r.Post("/api/v1/locations", handler.Create)
	r.Put("/api/v1/locations/{id}", handler.Update)
	r.Delete("/api/v1/locations/{id}", handler.Delete)
	r.Post("/api/v1/locations/{id}/identifiers", handler.AddIdentifier)
	r.Delete("/api/v1/locations/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
	r.Get("/api/v1/locations/{id}/ancestors", handler.GetAncestors)
	r.Get("/api/v1/locations/{id}/descendants", handler.GetDescendants)
	r.Get("/api/v1/locations/{id}/children", handler.GetChildren)
}
