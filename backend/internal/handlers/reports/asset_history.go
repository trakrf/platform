package reports

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

const (
	assetHistoryDefaultLimit = 50
	assetHistoryMaxLimit     = 100
	defaultDateRangeDays     = 30
)

// GetAssetHistory handles GET /api/v1/reports/assets/{id}/history
// @Summary Get asset location history
// @Description Get paginated location history for a single asset with duration calculations
// @Tags reports
// @Accept json
// @Produce json
// @Param id path int true "Asset ID"
// @Param limit query int false "Results per page (default 50, max 100)" minimum(1) maximum(100) default(50)
// @Param offset query int false "Pagination offset (default 0)" minimum(0) default(0)
// @Param start_date query string false "Filter scans after this time (ISO 8601)"
// @Param end_date query string false "Filter scans before this time (ISO 8601)"
// @Success 200 {object} report.AssetHistoryResponse
// @Failure 400 {object} modelerrors.ErrorResponse "Invalid asset ID or date format"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 404 {object} modelerrors.ErrorResponse "Asset not found"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/reports/assets/{id}/history [get]
func (h *Handler) GetAssetHistory(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	// 1. Get org from claims
	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.ReportAssetHistoryFailed, "missing organization context", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	// 2. Parse path param: id (asset ID)
	idParam := chi.URLParam(r, "id")
	assetID, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.ReportInvalidAssetID, idParam), err.Error(), requestID)
		return
	}

	// 3. Validate asset exists and belongs to org
	asset, err := h.storage.GetAssetByID(r.Context(), &assetID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryFailed, err.Error(), requestID)
		return
	}
	if asset == nil || asset.OrgID != orgID {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.ReportAssetNotFound, "asset not found or not accessible", requestID)
		return
	}

	// 4. Parse query parameters
	filter := report.AssetHistoryFilter{
		Limit:  assetHistoryDefaultLimit,
		Offset: 0,
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			filter.Limit = parsed
			if filter.Limit > assetHistoryMaxLimit {
				filter.Limit = assetHistoryMaxLimit
			}
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			filter.Offset = parsed
		}
	}

	// Default date range: last 30 days
	now := time.Now()
	defaultStart := now.AddDate(0, 0, -defaultDateRangeDays)
	filter.StartDate = &defaultStart
	filter.EndDate = &now

	if startDateStr := r.URL.Query().Get("start_date"); startDateStr != "" {
		parsed, err := time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.ReportInvalidDateFormat, "invalid start_date format, use RFC3339", requestID)
			return
		}
		filter.StartDate = &parsed
	}

	if endDateStr := r.URL.Query().Get("end_date"); endDateStr != "" {
		parsed, err := time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				apierrors.ReportInvalidDateFormat, "invalid end_date format, use RFC3339", requestID)
			return
		}
		filter.EndDate = &parsed
	}

	// 5. Get active identifier for asset
	identifier := ""
	identifiers, err := h.storage.GetIdentifiersByAssetID(r.Context(), assetID)
	if err == nil {
		for _, id := range identifiers {
			if id.IsActive {
				identifier = id.Value
				break
			}
		}
	}

	// 6. Fetch history data
	items, err := h.storage.ListAssetHistory(r.Context(), assetID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryFailed, err.Error(), requestID)
		return
	}

	totalCount, err := h.storage.CountAssetHistory(r.Context(), assetID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryCount, err.Error(), requestID)
		return
	}

	// 7. Return response
	response := report.AssetHistoryResponse{
		Asset: report.AssetInfo{
			ID:         asset.ID,
			Name:       asset.Name,
			Identifier: identifier,
		},
		Data:       items,
		Count:      len(items),
		Offset:     filter.Offset,
		TotalCount: totalCount,
	}

	httputil.WriteJSON(w, http.StatusOK, response)
}
