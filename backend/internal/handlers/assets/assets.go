package assets

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/asset"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = validator.New()

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

// @Summary Create asset
// @Description Create a new asset
// @Tags assets
// @Accept json
// @Produce json
// @Param request body asset.Asset true "Asset to create"
// @Success 201 {object} map[string]any "data: asset.Asset"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid JSON or validation error"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/assets [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var request asset.Asset
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid JSON", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			"Validation failed", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	result, err := handler.storage.CreateAsset(r.Context(), request)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to create asset", err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	w.Header().Set("Location", "/api/v1/assets/"+strconv.Itoa(result.ID))
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// @Summary Update asset
// @Description Update an existing asset by ID
// @Tags assets
// @Accept json
// @Produce json
// @Param id path int true "Asset ID"
// @Param request body asset.UpdateAssetRequest true "Asset update data"
// @Success 202 {object} map[string]any "data: asset.Asset"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid ID, JSON, or validation error"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/assets/{id} [put]
func (handler *Handler) UpdateAsset(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())
	idParam := chi.URLParam(req, "id")

	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf("Invalid Asset ID: %s", idParam), err.Error(), ctx)
		return
	}

	var request asset.UpdateAssetRequest
	

	if err:= json.NewDecoder(req.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest, "Invalid Request", err.Error(), ctx)
		return
	}

	if err:= validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrValidation, "Validation Failed", err.Error(), ctx)
		return
	}

	result, err := handler.storage.UpdateAsset(req.Context(), id, request)

	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal, "Failed to update asset", err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]*asset.Asset{"data": result})
}

// @Summary Get asset
// @Description Get an asset by ID
// @Tags assets
// @Accept json
// @Produce json
// @Param id path int true "Asset ID"
// @Success 202 {object} map[string]any "data: asset.Asset"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid asset ID"
// @Failure 404 {object} modelerrors.ErrorResponse "Asset not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/assets/{id} [get]
func (handler *Handler) GetAsset(w http.ResponseWriter, req *http.Request) {
	idParam := chi.URLParam(req, "id")
	ctx := middleware.GetRequestID(req.Context())

	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf("Invalid Asset ID: %s", idParam), err.Error(), middleware.GetRequestID(req.Context()))
		return
	}

	result, err := handler.storage.GetAssetByID(req.Context(), &id)

	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal, "Failed to get asset", err.Error(), ctx)
		return
	}
	
	if result == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			"Asset not found", "", ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]*asset.Asset{"data": result})
}

// @Summary Delete asset
// @Description Soft delete an asset by ID
// @Tags assets
// @Accept json
// @Produce json
// @Param id path int true "Asset ID"
// @Success 202 {object} map[string]bool "deleted: true/false"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid asset ID"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/assets/{id} [delete]
func (handler *Handler) DeleteAsset(w http.ResponseWriter, req *http.Request) {
	idParam := chi.URLParam(req, "id")
	ctx := middleware.GetRequestID(req.Context())

	id, err := strconv.Atoi(idParam)

	if err!= nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf("Invalid Asset ID: %s", idParam), err.Error(), ctx)
		return
	}

	deleted, err := handler.storage.DeleteAsset(req.Context(), &id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal, "Failed to delete asset", err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]bool{"deleted": deleted})
}

// @Summary List assets
// @Description Get a paginated list of all assets
// @Tags assets
// @Accept json
// @Produce json
// @Success 202 {object} map[string]any "data: []asset.Asset"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/assets [get]
func (handler *Handler) ListAssets(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

	assets, err := handler.storage.ListAllAssets(req.Context(), 10, 0)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal, "Failed to list assets", err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string][]asset.Asset{"data": assets})
}

// RegisterRoutes registers all asset routes
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/assets", handler.ListAssets)
	r.Get("/api/v1/assets/{id}", handler.GetAsset)
	r.Post("/api/v1/assets", handler.Create)
	r.Put("/api/v1/assets/{id}", handler.UpdateAsset)
	r.Delete("/api/v1/assets/{id}", handler.DeleteAsset)
	r.Get("/api/v1/assets/bulk/{jobId}", handler.GetJobStatus)
}