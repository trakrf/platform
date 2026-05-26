package reports

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/apierrors"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

const (
	defaultDateRangeDays = 30
)

// respondInvalidTimestamp writes a 400 validation_error envelope naming the
// offending query parameter.
func respondInvalidTimestamp(w http.ResponseWriter, r *http.Request, field, reqID string) {
	msg := fmt.Sprintf("Invalid '%s' timestamp; expected RFC 3339, e.g. 2026-04-21T00:00:00.000Z", field)
	httputil.WriteValidationError(w, r, reqID, []modelerrors.FieldError{{
		Field:   field,
		Code:    "invalid_value",
		Message: msg,
	}})

}

// AssetHistoryResponse is the typed envelope returned by
// GET /api/v1/assets/{asset_id}/history. The body shape (report.PublicAssetHistoryItem)
// is owned by the Reports rename child of TRA-549, not by TRA-555.
type AssetHistoryResponse struct {
	Data       []report.PublicAssetHistoryItem `json:"data"`
	Limit      int                             `json:"limit"       example:"50"`
	Offset     int                             `json:"offset"      example:"0"`
	TotalCount int                             `json:"total_count" example:"100"`
}

// @Summary Asset movement history
// @Description Location history for an asset identified by its canonical id.
// @Description
// @Description The asset existence check follows path-addressed semantics — the asset is returned even if its `valid_to` has elapsed. Each history row's location reference applies the temporal-validity predicate, so an event referencing a location whose effective window is past surfaces with null `location_external_key`.
// @Tags assets,public
// @ID assets.history
// @Param asset_id path int true "Asset id (canonical)" minimum(1)
// @Param limit query int false "max 200"   default(50) minimum(1) maximum(200)
// @Param offset query int false "min 0"    default(0) minimum(0)
// @Param from query string false "RFC 3339 start timestamp" format(date-time)
// @Param to query string false "RFC 3339 end timestamp" format(date-time)
// @Param sort query []string false "comma-separated; prefix '-' for DESC" collectionFormat(csv) Enums(event_observed_at, -event_observed_at)
// @Success 200 {object} reports.AssetHistoryResponse
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
// @Router /api/v1/assets/{asset_id}/history [get]
func (h *Handler) GetAssetHistory(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}

	id, err := httputil.ParseSurrogateID("asset_id", chi.URLParam(r, "asset_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}

	assetRow, err := h.storage.GetAssetByID(r.Context(), orgID, &id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}
	if assetRow == nil || assetRow.OrgID != orgID {
		httputil.Respond404(w, r, apierrors.ReportAssetNotFound, reqID)
		return
	}

	params, err := httputil.ParseListParams(r, httputil.ListAllowlist{
		Filters: []string{"from", "to"},
		Sorts:   []string{"event_observed_at"},
	})
	if err != nil {
		httputil.RespondListParamError(w, r, err, reqID)
		return
	}

	filter := report.AssetHistoryFilter{Limit: params.Limit, Offset: params.Offset}
	if vs, ok := params.Filters["from"]; ok && len(vs) > 0 {
		t, err := time.Parse(time.RFC3339Nano, vs[0])
		if err != nil {
			respondInvalidTimestamp(w, r, "from", reqID)
			return
		}
		filter.From = &t
	}
	if vs, ok := params.Filters["to"]; ok && len(vs) > 0 {
		t, err := time.Parse(time.RFC3339Nano, vs[0])
		if err != nil {
			respondInvalidTimestamp(w, r, "to", reqID)
			return
		}
		filter.To = &t
	}
	for _, s := range params.Sorts {
		filter.Sorts = append(filter.Sorts, report.AssetHistorySort{Field: s.Field, Desc: s.Desc})
	}

	items, err := h.storage.ListAssetHistory(r.Context(), assetRow.ID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}
	total, err := h.storage.CountAssetHistory(r.Context(), assetRow.ID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			err.Error(), reqID)

		return
	}

	out := make([]report.PublicAssetHistoryItem, 0, len(items))
	for _, it := range items {
		out = append(out, report.ToPublicAssetHistoryItem(it))
	}

	httputil.WriteJSON(w, http.StatusOK, AssetHistoryResponse{
		Data:       out,
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}
