package inventory

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
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
	GetLocationByExternalKey(ctx context.Context, orgID int, identifier string) (*location.LocationWithParent, error)
	GetAssetIDsByExternalKeys(ctx context.Context, orgID int, externalKeys []string) (map[string]int, error)
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
// Both fields are required; the public surface has a single canonical shape
// (TRA-533). Use natural identifiers — surrogate IDs were removed to collapse
// the C2-class spelling proliferation flagged in TRA-532 finding F10.
type SaveRequest struct {
	LocationIdentifier *string  `json:"location_identifier" validate:"required,min=1,max=255" example:"WH-01"`
	AssetIdentifiers   []string `json:"asset_identifiers" validate:"required,min=1,dive,min=1,max=255" example:"ASSET-0001"`
}

// SaveResponse is the typed envelope returned on success by POST /api/v1/inventory/save.
type SaveResponse struct {
	Data storage.SaveInventoryResult `json:"data"`
}

// Save handles POST /api/v1/inventory/save
// @Summary Save inventory scans
// @Description Persist scanned RFID assets to the asset_scans hypertable
// @Tags inventory,internal
// @ID inventory.save
// @Accept json
// @Produce json
// @Param request body SaveRequest true "Save request with location and asset identifiers"
// @Success 201 {object} inventory.SaveResponse
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid request"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 403 {object} modelerrors.ErrorResponse "Location or assets not owned by org"
// @Failure 415 {object} modelerrors.ErrorResponse "unsupported_media_type"
// @Failure 429 {object} modelerrors.ErrorResponse "rate_limited"
// @Header  429 {integer} Retry-After           "Seconds to wait before retrying"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth[scans:write]
// @Router /api/v1/inventory/save [post]
func (h *Handler) Save(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, requestID)
		return
	}

	var request SaveRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	// Resolve location_identifier → numeric.
	loc, err := h.storage.GetLocationByExternalKey(r.Context(), orgID, *request.LocationIdentifier)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}
	if loc == nil {
		msg := fmt.Sprintf("location_identifier %q not found", *request.LocationIdentifier)
		httputil.WriteValidationError(w, r, requestID, []modelerrors.FieldError{{
			Field:   "location_identifier",
			Code:    "invalid_value",
			Message: msg,
		}})

		return
	}
	locationID := loc.ID

	// Resolve asset_identifiers → numeric IDs (one query).
	resolved, err := h.storage.GetAssetIDsByExternalKeys(r.Context(), orgID, request.AssetIdentifiers)
	if err != nil {
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}
	assetIDs := make([]int, 0, len(request.AssetIdentifiers))
	var missing []string
	for _, ident := range request.AssetIdentifiers {
		if id, ok := resolved[ident]; ok {
			assetIDs = append(assetIDs, id)
		} else {
			missing = append(missing, ident)
		}
	}
	if len(missing) > 0 {
		fields := make([]modelerrors.FieldError, 0, len(missing))
		for _, m := range missing {
			fields = append(fields, modelerrors.FieldError{
				Field:   "asset_identifiers",
				Code:    "invalid_value",
				Message: fmt.Sprintf("asset_identifier %q not found", m),
			})
		}
		httputil.WriteValidationError(w, r, requestID, fields)

		return
	}

	result, err := h.storage.SaveInventoryScans(r.Context(), orgID, storage.SaveInventoryRequest{
		LocationID: locationID,
		AssetIDs:   assetIDs,
	})

	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "not found or access denied") {
			logger.Get().Warn().
				Int("org_id", orgID).
				Int("location_id", locationID).
				Ints("asset_ids", assetIDs).
				Str("request_id", requestID).
				Str("error", errStr).
				Msg("Inventory save denied: org context mismatch")

			httputil.WriteJSONError(w, r, http.StatusForbidden, modelerrors.ErrForbidden,
				errStr, requestID)

			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// RegisterRoutes is intentionally empty — POST /api/v1/inventory/save is
// registered in internal/cmd/serve/router.go under the public write group
// (EitherAuth + WriteAudit + RequireScope("scans:write")).
func (h *Handler) RegisterRoutes(r chi.Router) {}
