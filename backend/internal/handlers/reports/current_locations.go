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
// GET /api/v1/reports/asset-locations.
type ListCurrentLocationsResponse struct {
	Data       []report.PublicCurrentLocationItem `json:"data"`
	Limit      int                                `json:"limit"       example:"50"`
	Offset     int                                `json:"offset"      example:"0"`
	TotalCount int                                `json:"total_count" example:"100"`
}

// @Summary List current asset locations
// @Description Snapshot of each asset's most recent location, filterable by either side of the join. Filter by location (`location_id` / `location_external_key`) to retrieve everything currently at a place; filter by asset (`asset_id` / `asset_external_key`, repeatable) to resolve a batch of assets from a master system to their current locations in one round-trip. Within each pair the surrogate and natural-key forms are mutually exclusive (400 `ambiguous_fields` if both are supplied); the asset and location filter pairs are independent and intersect when combined. Because this view is derived from immutable scan history, it can resolve references for assets that have since been deleted. By default those rows are excluded; pass `include_deleted=true` to include them, and check `asset_deleted_at` to distinguish deleted from live.
// @Description
// @Description Rows are produced from `scan_event` history and reflect the most recent observed location per asset. **Assets that have never been scanned do not appear in this report** — they exist in `/api/v1/assets` but have no derived location row until at least one scan event has been observed, so this endpoint's `total_count` can lag `/api/v1/assets` `total_count` for newly-onboarded inventory. Use `/api/v1/assets` directly if you need a complete asset roster including never-scanned assets.
// @Description
// @Description Temporal validity is applied to both joined entities. Assets whose effective window is past or future are excluded entirely. Locations whose effective window is past or future surface with null `location_id` / `location_external_key` while the parent asset row remains visible. Soft-deleted locations are projected the same way here — null on the report row — even though the identifier still lives on the location row; reports endpoints intentionally hide tombstoned anchor points from scan-derived summaries. Use the locations endpoint with `include_deleted=true` to retrieve the underlying identifier.
// @Tags reports,public
// @ID reports.asset-locations
// @Param limit                 query int    false "max 200"   default(50) minimum(1) maximum(200)
// @Param offset                query int    false "min 0"    default(0) minimum(0)
// @Param location_id           query []int    false "filter by location id (canonical, may repeat); mutually exclusive with location_external_key (400 ambiguous_fields if both supplied)" collectionFormat(multi)
// @Param location_external_key query []string false "filter by location external_key (may repeat); mutually exclusive with location_id (400 ambiguous_fields if both supplied)" collectionFormat(multi)
// @Param asset_id              query []int    false "filter by asset id (canonical, may repeat); mutually exclusive with asset_external_key (400 ambiguous_fields if both supplied)" collectionFormat(multi)
// @Param asset_external_key    query []string false "filter by asset external_key (may repeat); mutually exclusive with asset_id (400 ambiguous_fields if both supplied)" collectionFormat(multi)
// @Param q                     query string false "substring search (case-insensitive) on asset name, external_key, and active tag values"
// @Param include_deleted       query bool   false "include rows for soft-deleted assets" default(false)
// @Param sort                  query []string false "comma-separated sort fields; prefix '-' for DESC" collectionFormat(csv) Enums(asset_last_seen, -asset_last_seen, asset_external_key, -asset_external_key, location_external_key, -location_external_key)
// @Success 200 {object} reports.ListCurrentLocationsResponse
// @Header  200 {integer} X-RateLimit-Limit     "Steady-state requests/min for this API key"
// @Header  200 {integer} X-RateLimit-Remaining "Requests remaining before throttling; bounded by X-RateLimit-Limit"
// @Header  200 {integer} X-RateLimit-Reset     "Unix timestamp (seconds) when X-RateLimit-Remaining will next equal X-RateLimit-Limit"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 429  {object}  modelerrors.ErrorResponse     "rate_limited"
// @Header  429 {integer} Retry-After           "Seconds to wait before retrying"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth[tracking:read]
// @Router /api/v1/reports/asset-locations [get]
func (h *Handler) ListCurrentLocations(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}

	params, err := httputil.ParseListParams(r, httputil.ListAllowlist{
		Filters:     []string{"location_id", "location_external_key", "asset_id", "asset_external_key", "q", "include_deleted"},
		BoolFilters: []string{"include_deleted"},
		// TRA-641 / BB21 §2.6: sort keys renamed to match the spec convention
		// used elsewhere — natural-key columns are addressed as
		// `<entity>_external_key`, never the bare entity name.
		Sorts: []string{"asset_last_seen", "asset_external_key", "location_external_key"},
	})
	if err != nil {
		httputil.RespondListParamError(w, r, err, reqID)
		return
	}

	// TRA-681 / TRA-735: each (id, external_key) pair on this endpoint is a
	// oneOf; supplying both forms of the same entity returns 400
	// ambiguous_fields. The asset and location pairs are independent —
	// combining one asset filter with one location filter intersects.
	_, hasLocID := params.Filters["location_id"]
	_, hasLocExt := params.Filters["location_external_key"]
	if hasLocID && hasLocExt {
		httputil.WriteValidationError(w, r, reqID, []modelerrors.FieldError{
			{Field: "location_id", Code: "ambiguous_fields", Message: "location_id and location_external_key are mutually exclusive; supply exactly one"},
			{Field: "location_external_key", Code: "ambiguous_fields", Message: "location_id and location_external_key are mutually exclusive; supply exactly one"},
		})
		return
	}
	_, hasAssetID := params.Filters["asset_id"]
	_, hasAssetExt := params.Filters["asset_external_key"]
	if hasAssetID && hasAssetExt {
		httputil.WriteValidationError(w, r, reqID, []modelerrors.FieldError{
			{Field: "asset_id", Code: "ambiguous_fields", Message: "asset_id and asset_external_key are mutually exclusive; supply exactly one"},
			{Field: "asset_external_key", Code: "ambiguous_fields", Message: "asset_id and asset_external_key are mutually exclusive; supply exactly one"},
		})
		return
	}

	// TRA-713 / BB33 F5+C2: external_key-style filters must enforce the
	// same regex the field validators apply on POST/PATCH. Without this,
	// a slash-containing (or otherwise non-conforming) value silently
	// returns 200-with-empty rather than 400 invalid_value, masking
	// integration bugs at the boundary.
	if fe := httputil.ValidateExternalKeyFilterValues("location_external_key", params.Filters["location_external_key"]); fe != nil {
		httputil.WriteValidationError(w, r, reqID, []modelerrors.FieldError{*fe})
		return
	}
	if fe := httputil.ValidateExternalKeyFilterValues("asset_external_key", params.Filters["asset_external_key"]); fe != nil {
		httputil.WriteValidationError(w, r, reqID, []modelerrors.FieldError{*fe})
		return
	}

	filter := report.CurrentLocationFilter{
		LocationExternalKeys: params.Filters["location_external_key"],
		AssetExternalKeys:    params.Filters["asset_external_key"],
		Limit:                params.Limit,
		Offset:               params.Offset,
	}
	if vs, ok := params.Filters["location_id"]; ok && len(vs) > 0 {
		filter.LocationIDs = make([]int, 0, len(vs))
		for _, s := range vs {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 {
				httputil.WriteValidationError(w, r, reqID, []modelerrors.FieldError{{
					Field:   "location_id",
					Code:    "invalid_value",
					Message: fmt.Sprintf("location_id %q must be a positive integer", s),
				}})

				return
			}
			filter.LocationIDs = append(filter.LocationIDs, n)
		}
	}
	if vs, ok := params.Filters["asset_id"]; ok && len(vs) > 0 {
		filter.AssetIDs = make([]int, 0, len(vs))
		for _, s := range vs {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 {
				httputil.WriteValidationError(w, r, reqID, []modelerrors.FieldError{{
					Field:   "asset_id",
					Code:    "invalid_value",
					Message: fmt.Sprintf("asset_id %q must be a positive integer", s),
				}})

				return
			}
			filter.AssetIDs = append(filter.AssetIDs, n)
		}
	}
	if vs, ok := params.Filters["q"]; ok && len(vs) > 0 {
		filter.Q = &vs[0]
	}
	if vs, ok := params.Filters["include_deleted"]; ok && len(vs) > 0 {
		filter.IncludeDeleted = vs[0] == "true"
	}
	for _, s := range params.Sorts {
		filter.Sorts = append(filter.Sorts, report.CurrentLocationSort{Field: s.Field, Desc: s.Desc})
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
