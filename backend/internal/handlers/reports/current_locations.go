package reports

import (
	"net/http"

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

// ListCurrentLocationsResponse is the typed envelope returned by
// GET /api/v1/locations/current.
type ListCurrentLocationsResponse struct {
	Data       []report.PublicCurrentLocationItem `json:"data"`
	Limit      int                                `json:"limit"       example:"50"`
	Offset     int                                `json:"offset"      example:"0"`
	TotalCount int                                `json:"total_count" example:"100"`
}

// @Summary List current asset locations
// @Description Snapshot of each asset's most recent location, filterable by natural key.
// @Tags reports,public
// @ID locations.current
// @Param limit query int false "max 200"
// @Param offset query int false "pagination offset"
// @Param location query string false "filter by location identifier (may repeat)"
// @Param q query string false "substring search (case-insensitive) on asset name, identifier, and active identifier values"
// @Param sort query string false "comma-separated sort fields; prefix '-' for DESC"
// @Success 200 {object} reports.ListCurrentLocationsResponse
// @Header  200 {integer} X-RateLimit-Limit     "Steady-state requests/min for this API key"
// @Header  200 {integer} X-RateLimit-Remaining "Requests remaining before throttling; bounded by X-RateLimit-Limit"
// @Header  200 {integer} X-RateLimit-Reset     "Unix timestamp (seconds) when X-RateLimit-Remaining will next equal X-RateLimit-Limit"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Header  429 {integer} Retry-After           "Seconds to wait before retrying"
// @Security APIKey[scans:read]
// @Router /api/v1/locations/current [get]
func (h *Handler) ListCurrentLocations(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.ReportCurrentLocationsFailed, "missing organization context", reqID)
		return
	}

	params, err := httputil.ParseListParams(r, httputil.ListAllowlist{
		Filters: []string{"location", "q"},
		Sorts:   []string{"last_seen", "asset", "location"},
	})
	if err != nil {
		httputil.RespondListParamError(w, r, err, reqID)
		return
	}

	filter := report.CurrentLocationFilter{
		LocationIdentifiers: params.Filters["location"],
		Limit:               params.Limit,
		Offset:              params.Offset,
	}
	if vs, ok := params.Filters["q"]; ok && len(vs) > 0 {
		filter.Q = &vs[0]
	}

	items, err := h.storage.ListCurrentLocations(r.Context(), orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportCurrentLocationsFailed, err.Error(), reqID)
		return
	}
	total, err := h.storage.CountCurrentLocations(r.Context(), orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportCurrentLocationsCount, err.Error(), reqID)
		return
	}

	out := make([]report.PublicCurrentLocationItem, 0, len(items))
	for _, it := range items {
		out = append(out, report.ToPublicCurrentLocationItem(it))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        out,
		"limit":       params.Limit,
		"offset":      params.Offset,
		"total_count": total,
	})
}

// RegisterRoutes is intentionally empty — reports routes are registered in
// internal/cmd/serve/router.go across the public and session-only groups.
func (h *Handler) RegisterRoutes(r chi.Router) {}
