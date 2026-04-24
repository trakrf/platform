package inventory

import (
	"context"
	"fmt"
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
	GetLocationByIdentifier(ctx context.Context, orgID int, identifier string) (*location.LocationWithParent, error)
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

	var request SaveRequest
	if err := httputil.DecodeJSON(r, &request); err != nil {
		httputil.RespondDecodeError(w, r, err, requestID)
		return
	}

	if err := validate.Struct(request); err != nil {
		httputil.RespondValidationError(w, r, err, requestID)
		return
	}

	// Cross-field: at least one of (location_id, location_identifier).
	hasLocID := request.LocationID > 0
	hasLocIdent := request.LocationIdentifier != nil && *request.LocationIdentifier != ""
	if !hasLocID && !hasLocIdent {
		msg := "location_identifier or location_id is required"
		httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.InventorySaveFailed, msg, requestID,
			[]modelerrors.FieldError{{
				Field:   "location_identifier",
				Code:    "required",
				Message: msg,
			}})
		return
	}

	// Cross-field: at least one of (asset_ids, asset_identifiers).
	hasAssetIDs := len(request.AssetIDs) > 0
	hasAssetIdents := len(request.AssetIdentifiers) > 0
	if !hasAssetIDs && !hasAssetIdents {
		msg := "asset_identifiers or asset_ids is required"
		httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.InventorySaveFailed, msg, requestID,
			[]modelerrors.FieldError{{
				Field:   "asset_identifiers",
				Code:    "required",
				Message: msg,
			}})
		return
	}
	if hasAssetIDs && hasAssetIdents {
		msg := "specify either asset_identifiers or asset_ids, not both"
		httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			apierrors.InventorySaveFailed, msg, requestID,
			[]modelerrors.FieldError{{
				Field:   "asset_identifiers",
				Code:    "invalid_value",
				Message: msg,
			}})
		return
	}

	// Resolve location_identifier → numeric.
	locationID := request.LocationID
	if hasLocIdent {
		loc, err := h.storage.GetLocationByIdentifier(r.Context(), orgID, *request.LocationIdentifier)
		if err != nil {
			httputil.RespondStorageError(w, r, err, requestID)
			return
		}
		if loc == nil {
			msg := fmt.Sprintf("location_identifier %q not found", *request.LocationIdentifier)
			httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
				apierrors.InventorySaveFailed, msg, requestID,
				[]modelerrors.FieldError{{
					Field:   "location_identifier",
					Code:    "invalid_value",
					Message: msg,
				}})
			return
		}
		if hasLocID && request.LocationID != loc.ID {
			msg := "location_identifier and location_id disagree"
			httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
				apierrors.InventorySaveFailed, msg, requestID,
				[]modelerrors.FieldError{{
					Field:   "location_identifier",
					Code:    "invalid_value",
					Message: msg,
				}})
			return
		}
		locationID = loc.ID
	}

	// Resolve asset_identifiers → numeric IDs (one query).
	assetIDs := request.AssetIDs
	if hasAssetIdents {
		resolved, err := h.storage.GetAssetIDsByIdentifiers(r.Context(), orgID, request.AssetIdentifiers)
		if err != nil {
			httputil.RespondStorageError(w, r, err, requestID)
			return
		}
		ids := make([]int, 0, len(request.AssetIdentifiers))
		var missing []string
		for _, ident := range request.AssetIdentifiers {
			if id, ok := resolved[ident]; ok {
				ids = append(ids, id)
			} else {
				missing = append(missing, ident)
			}
		}
		if len(missing) > 0 {
			msg := fmt.Sprintf("asset_identifier(s) not found: %s", strings.Join(missing, ", "))
			fields := make([]modelerrors.FieldError, 0, len(missing))
			for _, m := range missing {
				fields = append(fields, modelerrors.FieldError{
					Field:   "asset_identifiers",
					Code:    "invalid_value",
					Message: fmt.Sprintf("asset_identifier %q not found", m),
				})
			}
			httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
				apierrors.InventorySaveFailed, msg, requestID, fields)
			return
		}
		assetIDs = ids
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
				apierrors.InventorySaveForbidden, errStr, requestID)
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
