package assets

import (
	"context"
	"encoding/json"
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

func (handler *Handler) createAssetWithoutIdentifiers(ctx context.Context, request asset.CreateAssetWithIdentifiersRequest) (*asset.AssetView, error) {
	var validTo *time.Time
	if request.ValidTo != nil && !request.ValidTo.IsZero() {
		t := request.ValidTo.ToTime()
		validTo = &t
	}

	assetToCreate := asset.Asset{
		OrgID:             request.OrgID,
		Identifier:        request.Identifier,
		Name:              request.Name,
		Type:              request.Type,
		Description:       request.Description,
		CurrentLocationID: request.CurrentLocationID,
		ValidFrom:         request.ValidFrom.ToTime(),
		ValidTo:           validTo,
		Metadata:          request.Metadata,
		IsActive:          request.IsActive,
	}

	baseAsset, err := handler.storage.CreateAsset(ctx, assetToCreate)
	if err != nil {
		return nil, err
	}

	return &asset.AssetView{Asset: *baseAsset, Identifiers: []shared.TagIdentifier{}}, nil
}

// @Summary Create asset
// @Description Create a new asset, optionally with tag identifiers
// @Tags assets
// @Accept json
// @Produce json
// @Param request body asset.CreateAssetWithIdentifiersRequest true "Asset to create with optional identifiers"
// @Success 201 {object} map[string]any "data: asset.AssetView"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid JSON or validation error"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/assets [post]
func (handler *Handler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetCreateFailed, "missing organization context", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	var request asset.CreateAssetWithIdentifiersRequest
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

	request.OrgID = orgID

	var result *asset.AssetView
	var err error

	if len(request.Identifiers) > 0 {
		result, err = handler.storage.CreateAssetWithIdentifiers(r.Context(), request)
	} else {
		result, err = handler.createAssetWithoutIdentifiers(r.Context(), request)
	}

	if err != nil {
		// Check for duplicate identifier error (user error, not server error)
		if strings.Contains(err.Error(), "already exists") {
			httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.AssetCreateFailed, err.Error(), requestID)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetCreateFailed, err.Error(), requestID)
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
			fmt.Sprintf(apierrors.AssetUpdateInvalidID, idParam), err.Error(), ctx)
		return
	}

	var request asset.UpdateAssetRequest

	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.AssetUpdateInvalidReq, err.Error(), ctx)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.WriteJSONError(w, req, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.ValidationFailed, err.Error(), ctx)
		return
	}

	result, err := handler.storage.UpdateAsset(req.Context(), id, request)

	if err != nil {
		// Check for duplicate identifier error (user error, not server error)
		if strings.Contains(err.Error(), "already exists") {
			httputil.WriteJSONError(w, req, http.StatusConflict, modelerrors.ErrConflict,
				apierrors.AssetUpdateFailed, err.Error(), ctx)
			return
		}
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetUpdateFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]*asset.Asset{"data": result})
}

// @Summary Get asset
// @Description Get an asset by ID with its tag identifiers
// @Tags assets
// @Accept json
// @Produce json
// @Param id path int true "Asset ID"
// @Success 202 {object} map[string]any "data: asset.AssetView"
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
			fmt.Sprintf(apierrors.AssetGetInvalidID, idParam), err.Error(), middleware.GetRequestID(req.Context()))
		return
	}

	result, err := handler.storage.GetAssetViewByID(req.Context(), id)

	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetGetFailed, err.Error(), ctx)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.AssetNotFound, "", ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]*asset.AssetView{"data": result})
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
			fmt.Sprintf(apierrors.AssetDeleteInvalidID, idParam), err.Error(), ctx)
		return
	}

	deleted, err := handler.storage.DeleteAsset(req.Context(), &id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetDeleteFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]bool{"deleted": deleted})
}

type ListAssetsResponse struct {
	Data       []asset.AssetView `json:"data"`
	Count      int               `json:"count" example:"10"`
	Offset     int               `json:"offset" example:"0"`
	TotalCount int               `json:"total_count" example:"100"`
}

// @Summary List assets
// @Description Get a paginated list of all assets with their tag identifiers
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

	claims := middleware.GetUserClaims(req)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetListFailed, "missing organization context", ctx)
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

	assets, err := handler.storage.ListAssetViews(req.Context(), orgID, limit, offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetListFailed, err.Error(), ctx)
		return
	}

	totalCount, err := handler.storage.CountAllAssets(req.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetCountFailed, err.Error(), ctx)
		return
	}

	response := map[string]any{
		"data":        assets,
		"count":       len(assets),
		"offset":      offset,
		"total_count": totalCount,
	}

	httputil.WriteJSON(w, http.StatusAccepted, response)
}

// @Summary Add identifier to asset
// @Description Add a tag identifier (RFID, BLE, barcode) to an existing asset
// @Tags assets
// @Accept json
// @Produce json
// @Param id path int true "Asset ID"
// @Param request body shared.TagIdentifierRequest true "Tag identifier to add"
// @Success 201 {object} map[string]any "data: shared.TagIdentifier"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid request"
// @Failure 404 {object} modelerrors.ErrorResponse "Asset not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/assets/{id}/identifiers [post]
func (handler *Handler) AddIdentifier(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.AssetCreateFailed, "missing organization context", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	idParam := chi.URLParam(r, "id")
	assetID, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.AssetGetInvalidID, idParam), err.Error(), requestID)
		return
	}

	existingAsset, err := handler.storage.GetAssetByID(r.Context(), &assetID)
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

	identifier, err := handler.storage.AddIdentifierToAsset(r.Context(), orgID, assetID, request)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.AssetCreateFailed, err.Error(), requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": identifier})
}

// @Summary Remove identifier from asset
// @Description Remove a tag identifier from an asset
// @Tags assets
// @Accept json
// @Produce json
// @Param id path int true "Asset ID"
// @Param identifierId path int true "Identifier ID"
// @Success 202 {object} map[string]bool "deleted: true/false"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid request"
// @Failure 404 {object} modelerrors.ErrorResponse "Asset or identifier not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/assets/{id}/identifiers/{identifierId} [delete]
func (handler *Handler) RemoveIdentifier(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	idParam := chi.URLParam(r, "id")
	_, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.AssetGetInvalidID, idParam), err.Error(), requestID)
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
			apierrors.AssetDeleteFailed, err.Error(), requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]bool{"deleted": deleted})
}

func (handler *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/assets", handler.ListAssets)
	r.Get("/api/v1/assets/{id}", handler.GetAsset)
	r.Post("/api/v1/assets", handler.Create)
	r.Put("/api/v1/assets/{id}", handler.UpdateAsset)
	r.Delete("/api/v1/assets/{id}", handler.DeleteAsset)
	r.Post("/api/v1/assets/{id}/identifiers", handler.AddIdentifier)
	r.Delete("/api/v1/assets/{id}/identifiers/{identifierId}", handler.RemoveIdentifier)
	r.Post("/api/v1/assets/bulk", handler.UploadCSV)
	r.Get("/api/v1/assets/bulk/{jobId}", handler.GetJobStatus)
}
