package inventory

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = validator.New()

// Handler handles inventory-related API requests
type Handler struct {
	storage *storage.Storage
}

// NewHandler creates a new inventory handler
func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{
		storage: storage,
	}
}

// SaveRequest represents the request body for saving inventory scans
type SaveRequest struct {
	LocationID int   `json:"location_id" validate:"required"`
	AssetIDs   []int `json:"asset_ids" validate:"required,min=1"`
}

// Save handles POST /api/v1/inventory/save
// @Summary Save inventory scans
// @Description Persist scanned RFID assets to the asset_scans hypertable
// @Tags inventory
// @Accept json
// @Produce json
// @Param request body SaveRequest true "Save request with location and asset IDs"
// @Success 201 {object} map[string]any "data: SaveInventoryResult"
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid request"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "Location or assets not owned by org"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/inventory/save [post]
func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	// 1. Get org from claims
	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.InventorySaveFailed, "missing organization context", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	// 2. Decode and validate request
	var request SaveRequest
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

	// 3. Call storage layer
	result, err := h.storage.SaveInventoryScans(r.Context(), orgID, storage.SaveInventoryRequest{
		LocationID: request.LocationID,
		AssetIDs:   request.AssetIDs,
	})

	if err != nil {
		// Check for ownership validation errors (return 403)
		errStr := err.Error()
		if strings.Contains(errStr, "not found or access denied") {
			httputil.WriteJSONError(w, r, http.StatusForbidden, modelerrors.ErrForbidden,
				apierrors.InventorySaveForbidden, errStr, requestID)
			return
		}
		// Other errors are internal
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.InventorySaveFailed, err.Error(), requestID)
		return
	}

	// 4. Return success
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// RegisterRoutes registers inventory handler routes
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/v1/inventory/save", h.Save)
}
