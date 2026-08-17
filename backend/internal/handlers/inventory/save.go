package inventory

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

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

// MovedEvaluator detects asset.moved events from a completed save (TRA-1043);
// *assetevent.Evaluator satisfies it. Optional: a nil evaluator disables
// detection, which keeps the handler constructible in tests that do not care.
//
// It is invoked AFTER the save transaction commits. Since TRA-1118 the save
// splits into two event paths: assets whose save created a fresh minute-bucket
// row (InsertedAssetIDs) are evaluated normally against history, while assets
// whose save DO-UPDATEd an existing bucket to a different location
// (OverriddenFrom) are emitted with the explicit pre-save origin the upsert
// captured — the update destroyed it, so no lookup could recover it. A
// same-minute re-save at the SAME location appears in neither and stays
// silent.
type MovedEvaluator interface {
	EvaluateScans(ctx context.Context, orgID int, assetIDs []int, locationID int, at time.Time)
	EvaluateOverrides(ctx context.Context, orgID int, from map[int]*int, to int, at time.Time)
}

// Handler handles inventory-related API requests
type Handler struct {
	storage InventoryStorage
	moved   MovedEvaluator
}

// NewHandler creates a new inventory handler. moved may be nil.
func NewHandler(storage InventoryStorage, moved MovedEvaluator) *Handler {
	return &Handler{
		storage: storage,
		moved:   moved,
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
		var accessErr *storage.InventoryAccessError
		if errors.As(err, &accessErr) {
			// Log the structured bucket breakdown so a real cross-org leak or
			// concurrent soft-delete is distinguishable from the duplicate-id
			// path in the access log. The wire response stays generic (the
			// error string from accessErr.Error() does not include bucket IDs)
			// so callers cannot probe other orgs. (TRA-812)
			logger.Get().Warn().
				Int("org_id", orgID).
				Int("location_id", locationID).
				Ints("asset_ids", assetIDs).
				Str("reason", accessErr.Reason).
				Int("valid_count", accessErr.ValidCount).
				Int("total_count", accessErr.TotalCount).
				Ints("missing_asset_ids", accessErr.MissingAssetIDs).
				Ints("soft_deleted_asset_ids", accessErr.SoftDeletedAssetIDs).
				Ints("cross_org_asset_ids", accessErr.CrossOrgAssetIDs).
				Str("request_id", requestID).
				Str("error", accessErr.Error()).
				Msg("Inventory save denied")

			httputil.WriteJSONError(w, r, http.StatusForbidden, modelerrors.ErrForbidden,
				accessErr.Error(), requestID)

			return
		}
		httputil.RespondStorageError(w, r, err, requestID)
		return
	}

	// TRA-1043: asset.moved detection runs here, post-commit, and does its own
	// reads. Enqueueing from inside the write transaction would send a phantom
	// event whenever that transaction rolled back — the TRA-900 failure mode,
	// where the ingest fan-out silently rolled back because the org GUC was never
	// set. Post-commit is the only ordering that cannot lie.
	//
	// This is the handheld/manual Save path, which is what the real prod orgs
	// exercise today; a webhook covering only the fixed-reader path would look
	// broken to half the customer base.
	//
	// Best-effort by construction: detection failures are logged inside the
	// evaluator and the dispatcher drops rather than blocking, so a slow customer
	// endpoint can never delay a scan save.
	if h.moved != nil {
		if len(result.InsertedAssetIDs) > 0 {
			h.moved.EvaluateScans(r.Context(), orgID, result.InsertedAssetIDs, locationID, result.Timestamp)
		}
		if len(result.OverriddenFrom) > 0 {
			// Wall-clock time, not result.Timestamp: the bucket floor would sort
			// this correction BEFORE the same-minute reader event it overrides.
			h.moved.EvaluateOverrides(r.Context(), orgID, result.OverriddenFrom, locationID, time.Now())
		}
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// RegisterRoutes is intentionally empty — POST /api/v1/inventory/save is
// registered in internal/cmd/serve/router.go under the public write group
// (EitherAuth + WriteAudit + RequireScope("scans:write")).
func (h *Handler) RegisterRoutes(r chi.Router) {}
