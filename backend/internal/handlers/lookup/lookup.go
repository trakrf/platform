package lookup

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{
		storage: storage,
	}
}

// @Summary Lookup entity by tag
// @Description Find an asset or location by tag identifier value
// @Tags lookup
// @Accept json
// @Produce json
// @Param type query string true "Tag type (rfid, ble, barcode)"
// @Param value query string true "Tag value to search for"
// @Success 200 {object} map[string]any "data: storage.LookupResult"
// @Failure 400 {object} modelerrors.ErrorResponse "Missing required parameters"
// @Failure 404 {object} modelerrors.ErrorResponse "No entity found with this tag"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/lookup/tag [get]
func (h *Handler) LookupByTag(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LookupFailed, "missing organization context", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	tagType := r.URL.Query().Get("type")
	value := r.URL.Query().Get("value")

	if tagType == "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.LookupFailed, "type parameter is required", requestID)
		return
	}

	if value == "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.LookupFailed, "value parameter is required", requestID)
		return
	}

	result, err := h.storage.LookupByTagValue(r.Context(), orgID, tagType, value)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LookupFailed, err.Error(), requestID)
		return
	}

	if result == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LookupNotFound, "", requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// BatchLookupRequest is the request body for batch tag lookup
type BatchLookupRequest struct {
	Type   string   `json:"type"`   // e.g., "rfid"
	Values []string `json:"values"` // EPCs to lookup
}

// @Summary Batch lookup entities by tags
// @Description Find assets or locations by multiple tag identifier values
// @Tags lookup
// @Accept json
// @Produce json
// @Param request body BatchLookupRequest true "Batch lookup request"
// @Success 200 {object} map[string]any "data: map[string]*storage.LookupResult"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid request body or missing required fields"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/lookup/tags [post]
func (h *Handler) LookupByTags(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LookupFailed, "missing organization context", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	var req BatchLookupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.LookupFailed, "invalid request body", requestID)
		return
	}

	if req.Type == "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.LookupFailed, "type field is required", requestID)
		return
	}

	if len(req.Values) == 0 {
		// Return empty result for empty values
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{}})
		return
	}

	// Limit batch size to prevent abuse
	const maxBatchSize = 500
	if len(req.Values) > maxBatchSize {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			apierrors.LookupFailed, "batch size exceeds maximum of 500", requestID)
		return
	}

	results, err := h.storage.LookupByTagValues(r.Context(), orgID, req.Type, req.Values)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LookupFailed, err.Error(), requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": results})
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/lookup/tag", h.LookupByTag)
	r.Post("/api/v1/lookup/tags", h.LookupByTags)
}
