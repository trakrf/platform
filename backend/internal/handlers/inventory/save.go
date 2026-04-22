package inventory

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	return v
}()

// InventoryStorage defines the storage operations needed by the inventory handler.
type InventoryStorage interface {
	SaveInventoryScans(ctx context.Context, orgID int, req storage.SaveInventoryRequest) (*storage.SaveInventoryResult, error)
	GetLocationByIdentifier(ctx context.Context, orgID int, identifier string) (*location.Location, error)
	GetAssetIDsByIdentifiers(ctx context.Context, orgID int, identifiers []string) (map[string]int, error)
}

// Handler handles inventory-related API requests
type Handler struct {
	storage InventoryStorage
}

// NewHandler creates a new inventory handler
func NewHandler(storage InventoryStorage) *Handler {
	return &Handler{
		storage: storage,
	}
}

// SaveRequest is the request body for POST /api/v1/inventory/save.
//
// External API consumers should use location_identifier and asset_identifiers.
// The numeric location_id / asset_ids are accepted for backward compatibility
// with the UI (which already has surrogate IDs in client state) and are hidden
// from the public OpenAPI spec via swaggerignore.
//
// At least one of (location_id, location_identifier) and one of (asset_ids,
// asset_identifiers) must be provided. See Save handler for cross-field rules.
type SaveRequest struct {
	LocationID         int      `json:"location_id,omitempty" swaggerignore:"true" validate:"omitempty,min=1"`
	LocationIdentifier *string  `json:"location_identifier,omitempty" validate:"omitempty,min=1,max=255" example:"WH-01"`
	AssetIDs           []int    `json:"asset_ids,omitempty" swaggerignore:"true" validate:"omitempty,min=1,dive,min=1"`
	AssetIdentifiers   []string `json:"asset_identifiers,omitempty" validate:"omitempty,min=1,dive,min=1,max=255" example:"ASSET-0001"`
}

// SaveResponse is the typed envelope returned on success by POST /api/v1/inventory/save.
type SaveResponse struct {
	Data storage.SaveInventoryResult `json:"data"`
}

// Save handles POST /api/v1/inventory/save
// @Summary Save inventory scans
// @Description Persist scanned RFID assets to the asset_scans hypertable
// @Tags inventory,public
// @Accept json
// @Produce json
// @Param request body SaveRequest true "Save request with location and asset IDs"
// @Success 201 {object} inventory.SaveResponse
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid request"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "Location or assets not owned by org"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security APIKey[scans:write]
// @Router /api/v1/inventory/save [post]
func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.InventorySaveFailed, "missing organization context", requestID)
		return
	}

	// 2. Decode and validate request
	var request SaveRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	// 3. Call storage layer
	result, err := h.storage.SaveInventoryScans(r.Context(), orgID, storage.SaveInventoryRequest{
		LocationID: request.LocationID,
		AssetIDs:   request.AssetIDs,
	})

	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "not found or access denied") {
			logger.Get().Warn().
				Int("org_id", orgID).
				Int("location_id", request.LocationID).
				Ints("asset_ids", request.AssetIDs).
				Str("request_id", requestID).
				Str("error", errStr).
				Msg("Inventory save denied: org context mismatch")

			httputil.WriteJSONError(w, r, http.StatusForbidden, modelerrors.ErrForbidden,
				apierrors.InventorySaveForbidden, errStr, requestID)
			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	// 4. Return success
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// RegisterRoutes is intentionally empty — POST /api/v1/inventory/save is
// registered in internal/cmd/serve/router.go under the public write group
// (EitherAuth + WriteAudit + RequireScope("scans:write")).
func (h *Handler) RegisterRoutes(r chi.Router) {}
