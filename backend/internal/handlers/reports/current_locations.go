package reports

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

const (
	defaultLimit = 50
	maxLimit     = 100
)

// Handler handles report-related API requests
type Handler struct {
	storage *storage.Storage
}

// NewHandler creates a new reports handler
func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

// ListCurrentLocations handles GET /api/v1/reports/current-locations
// @Summary List current asset locations
// @Description Get paginated list of assets with their most recent location
// @Tags reports
// @Accept json
// @Produce json
// @Param limit query int false "Results per page (default 50, max 100)" minimum(1) maximum(100) default(50)
// @Param offset query int false "Pagination offset (default 0)" minimum(0) default(0)
// @Param location_id query int false "Filter by location ID"
// @Param search query string false "Search asset name or identifier"
// @Success 200 {object} report.CurrentLocationsResponse
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /api/v1/reports/current-locations [get]
func (h *Handler) ListCurrentLocations(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	// 1. Get org from claims
	claims := middleware.GetUserClaims(r)
	if claims == nil || claims.CurrentOrgID == nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.ReportCurrentLocationsFailed, "missing organization context", requestID)
		return
	}
	orgID := *claims.CurrentOrgID

	// 2. Parse query parameters
	filter := report.CurrentLocationFilter{
		Limit:  defaultLimit,
		Offset: 0,
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			filter.Limit = parsed
			if filter.Limit > maxLimit {
				filter.Limit = maxLimit
			}
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsed, err := strconv.Atoi(offsetStr); err == nil && parsed >= 0 {
			filter.Offset = parsed
		}
	}

	if locationIDStr := r.URL.Query().Get("location_id"); locationIDStr != "" {
		if parsed, err := strconv.Atoi(locationIDStr); err == nil {
			filter.LocationID = &parsed
		}
	}

	if search := r.URL.Query().Get("search"); search != "" {
		filter.Search = &search
	}

	// 3. Fetch data
	items, err := h.storage.ListCurrentLocations(r.Context(), orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportCurrentLocationsFailed, err.Error(), requestID)
		return
	}

	totalCount, err := h.storage.CountCurrentLocations(r.Context(), orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportCurrentLocationsCount, err.Error(), requestID)
		return
	}

	// 4. Return response
	response := report.CurrentLocationsResponse{
		Data:       items,
		Count:      len(items),
		Offset:     filter.Offset,
		TotalCount: totalCount,
	}

	httputil.WriteJSON(w, http.StatusOK, response)
}

// RegisterRoutes registers report handler routes
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/reports/current-locations", h.ListCurrentLocations)
	r.Get("/api/v1/reports/assets/{id}/history", h.GetAssetHistory)
}
