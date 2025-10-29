package assets

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/i18n"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/asset"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/services/bulkimport"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = validator.New()

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
			i18n.T("assets.create.invalid_json"), err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			i18n.T("assets.create.validation_failed"), err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	result, err := handler.storage.CreateAsset(r.Context(), request)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			i18n.T("assets.create.failed"), err.Error(), middleware.GetRequestID(r.Context()))
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
			i18n.T("assets.update.invalid_id", map[string]interface{}{"id": idParam}), err.Error(), ctx)
		return
	}

	var request asset.UpdateAssetRequest

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			i18n.T("assets.update.invalid_request"), err.Error(), ctx)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
			i18n.T("assets.update.validation_failed"), err.Error(), ctx)
		return
	}

	result, err := handler.storage.UpdateAsset(req.Context(), id, request)

	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			i18n.T("assets.update.failed"), err.Error(), ctx)
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
			i18n.T("assets.get.invalid_id", map[string]interface{}{"id": idParam}), err.Error(), middleware.GetRequestID(req.Context()))
		return
	}

	result, err := handler.storage.GetAssetByID(req.Context(), &id)

	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			i18n.T("assets.get.failed"), err.Error(), ctx)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			i18n.T("assets.get.not_found"), "", ctx)
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

	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			i18n.T("assets.delete.invalid_id", map[string]interface{}{"id": idParam}), err.Error(), ctx)
		return
	}

	deleted, err := handler.storage.DeleteAsset(req.Context(), &id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			i18n.T("assets.delete.failed"), err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]bool{"deleted": deleted})
}

type ListAssetsResponse struct {
	Data       []asset.Asset `json:"data"`
	Count      int           `json:"count" example:"10"`
	Offset     int           `json:"offset" example:"0"`
	TotalCount int           `json:"total_count" example:"100"`
}

// @Summary List assets
// @Description Get a paginated list of all assets with pagination metadata
// @Tags assets
// @Accept json
// @Produce json
// @Param limit query int false "Number of assets to return (default: 10)" minimum(1) default(10)
// @Param offset query int false "Number of assets to skip for pagination (default: 0)" minimum(0) default(0)
// @Success 202 {object} ListAssetsResponse "Paginated list of assets with metadata"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/assets [get]
func (handler *Handler) ListAssets(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())

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

	// Get paginated assets
	assets, err := handler.storage.ListAllAssets(req.Context(), limit, offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			i18n.T("assets.list.failed"), err.Error(), ctx)
		return
	}

	// Get total count for pagination metadata
	totalCount, err := handler.storage.CountAllAssets(req.Context())
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			i18n.T("assets.count.failed"), err.Error(), ctx)
		return
	}

	// Build response with pagination metadata
	response := map[string]any{
		"data":        assets,
		"count":       len(assets),
		"offset":      offset,
		"total_count": totalCount,
	}

	httputil.WriteJSON(w, http.StatusAccepted, response)
}

// RegisterRoutes registers all asset routes
func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/assets", handler.ListAssets)
	r.Get("/api/v1/assets/{id}", handler.GetAsset)
	r.Post("/api/v1/assets", handler.Create)
	r.Put("/api/v1/assets/{id}", handler.UpdateAsset)
	r.Delete("/api/v1/assets/{id}", handler.DeleteAsset)
	r.Post("/api/v1/assets/bulk", handler.UploadCSV)
	r.Get("/api/v1/assets/bulk/{jobId}", handler.GetJobStatus)
}
