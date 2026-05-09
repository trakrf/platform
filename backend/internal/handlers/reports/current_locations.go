package reports

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
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
// @Description Snapshot of each asset's most recent location, filterable by canonical id or external_key. Because this view is derived from immutable scan history, it can resolve references for assets that have since been deleted. By default those rows are excluded; pass `include_deleted=true` to include them, and check `asset_deleted_at` to distinguish deleted from live.
// @Description
// @Description Temporal validity is applied to both joined entities. Assets whose effective window is past or future are excluded entirely. Locations whose effective window is past or future surface with null `location_id` / `location_external_key` while the parent asset row remains visible — matching how soft-deleted locations are projected.
// @Tags locations,public
// @ID locations.current
// @Param limit                 query int    false "max 200"   default(50) minimum(1) maximum(200)
// @Param offset                query int    false "min 0"    default(0) minimum(0)
// @Param location_id           query []int    false "filter by location id (canonical, may repeat)" collectionFormat(multi)
// @Param location_external_key query []string false "filter by location external_key (may repeat)" collectionFormat(multi)
// @Param q                     query string false "substring search (case-insensitive) on asset name, external_key, and active tag values"
// @Param include_deleted       query bool   false "include rows for soft-deleted assets" default(false)
// @Param sort                  query []string false "comma-separated sort fields; prefix '-' for DESC" collectionFormat(csv) Enums(last_seen, -last_seen, asset, -asset, location, -location)
// @Success 200 {object} reports.ListCurrentLocationsResponse
// @Header  200 {integer} X-RateLimit-Limit     "Steady-state requests/min for this API key"
// @Header  200 {integer} X-RateLimit-Remaining "Requests remaining before throttling; bounded by X-RateLimit-Limit"
// @Header  200 {integer} X-RateLimit-Reset     "Unix timestamp (seconds) when X-RateLimit-Remaining will next equal X-RateLimit-Limit"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Header  429 {integer} Retry-After           "Seconds to wait before retrying"
// @Security BearerAuth[history:read]
// @Router /api/v1/locations/current [get]
func (h *Handler) ListCurrentLocations(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}

	params, err := httputil.ParseListParams(r, httputil.ListAllowlist{
		Filters:     []string{"location_id", "location_external_key", "q", "include_deleted"},
		BoolFilters: []string{"include_deleted"},
		Sorts:       []string{"last_seen", "asset", "location"},
	})
	if err != nil {
		httputil.RespondListParamError(w, r, err, reqID)
		return
	}

	filter := report.CurrentLocationFilter{
		LocationExternalKeys: params.Filters["location_external_key"],
		Limit:                params.Limit,
		Offset:               params.Offset,
	}
	if vs, ok := params.Filters["location_id"]; ok && len(vs) > 0 {
		filter.LocationIDs = make([]int, 0, len(vs))
		for _, s := range vs {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 {
				httputil.WriteJSONErrorWithFields(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
					"invalid location_id", reqID,
					[]modelerrors.FieldError{{
						Field:   "location_id",
						Code:    "invalid_value",
						Message: fmt.Sprintf("location_id %q must be a positive integer", s),
					}})

				return
			}
			filter.LocationIDs = append(filter.LocationIDs, n)
		}
	}
	if vs, ok := params.Filters["q"]; ok && len(vs) > 0 {
		filter.Q = &vs[0]
	}
	if vs, ok := params.Filters["include_deleted"]; ok && len(vs) > 0 {
		filter.IncludeDeleted = vs[0] == "true"
	}

	items, err := h.storage.ListCurrentLocations(r.Context(), orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}
	total, err := h.storage.CountCurrentLocations(r.Context(), orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	out := make([]report.PublicCurrentLocationItem, 0, len(items))
	for _, it := range items {
		out = append(out, report.ToPublicCurrentLocationItem(it))
	}

	httputil.WriteJSON(w, http.StatusOK, ListCurrentLocationsResponse{
		Data:       out,
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}

// RegisterRoutes is intentionally empty — reports routes are registered in
// internal/cmd/serve/router.go across the public and session-only groups.
func (h *Handler) RegisterRoutes(r chi.Router) {}
