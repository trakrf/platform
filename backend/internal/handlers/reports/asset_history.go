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

// GetAssetHistory serves /api/v1/assets/{identifier}/history.
func (h *Handler) GetAssetHistory(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.ReportAssetHistoryFailed, "missing organization context", reqID)
		return
	}

	identifier := chi.URLParam(r, "identifier")
	assetRow, err := h.storage.GetAssetByIdentifier(r.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryFailed, err.Error(), reqID)
		return
	}
	if assetRow == nil {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.ReportAssetNotFound, "asset not found", reqID)
		return
	}

	params, err := httputil.ParseListParams(r, httputil.ListAllowlist{
		Filters: []string{"from", "to"},
		Sorts:   []string{"timestamp"},
	})
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid list parameters", err.Error(), reqID)
		return
	}

	filter := report.AssetHistoryFilter{Limit: params.Limit, Offset: params.Offset}
	if vs, ok := params.Filters["from"]; ok && len(vs) > 0 {
		t, err := time.Parse(time.RFC3339, vs[0])
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				"Invalid 'from' timestamp; RFC3339 required", err.Error(), reqID)
			return
		}
		filter.From = &t
	}
	if vs, ok := params.Filters["to"]; ok && len(vs) > 0 {
		t, err := time.Parse(time.RFC3339, vs[0])
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				"Invalid 'to' timestamp; RFC3339 required", err.Error(), reqID)
			return
		}
		filter.To = &t
	}

	items, err := h.storage.ListAssetHistory(r.Context(), assetRow.ID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryFailed, err.Error(), reqID)
		return
	}
	total, err := h.storage.CountAssetHistory(r.Context(), assetRow.ID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryCount, err.Error(), reqID)
		return
	}

	out := make([]report.PublicAssetHistoryItem, 0, len(items))
	for _, it := range items {
		out = append(out, report.ToPublicAssetHistoryItem(it))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        out,
		"limit":       params.Limit,
		"offset":      params.Offset,
		"total_count": total,
	})
}

// GetAssetHistoryByID serves /api/v1/assets/by-id/{id}/history.
func (h *Handler) GetAssetHistoryByID(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())

	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.ReportAssetHistoryFailed, "missing organization context", reqID)
		return
	}

	idParam := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			fmt.Sprintf(apierrors.ReportInvalidAssetID, idParam), err.Error(), reqID)
		return
	}

	assetRow, err := h.storage.GetAssetByID(r.Context(), &id)
	if err != nil || assetRow == nil || assetRow.OrgID != orgID {
		httputil.WriteJSONError(w, r, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.ReportAssetNotFound, "asset not found or not accessible", reqID)
		return
	}

	params, err := httputil.ParseListParams(r, httputil.ListAllowlist{
		Filters: []string{"from", "to"},
		Sorts:   []string{"timestamp"},
	})
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid list parameters", err.Error(), reqID)
		return
	}

	filter := report.AssetHistoryFilter{Limit: params.Limit, Offset: params.Offset}
	if vs, ok := params.Filters["from"]; ok && len(vs) > 0 {
		t, err := time.Parse(time.RFC3339, vs[0])
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				"Invalid 'from' timestamp; RFC3339 required", err.Error(), reqID)
			return
		}
		filter.From = &t
	}
	if vs, ok := params.Filters["to"]; ok && len(vs) > 0 {
		t, err := time.Parse(time.RFC3339, vs[0])
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
				"Invalid 'to' timestamp; RFC3339 required", err.Error(), reqID)
			return
		}
		filter.To = &t
	}

	items, err := h.storage.ListAssetHistory(r.Context(), assetRow.ID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryFailed, err.Error(), reqID)
		return
	}
	total, err := h.storage.CountAssetHistory(r.Context(), assetRow.ID, orgID, filter)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.ReportAssetHistoryCount, err.Error(), reqID)
		return
	}

	out := make([]report.PublicAssetHistoryItem, 0, len(items))
	for _, it := range items {
		out = append(out, report.ToPublicAssetHistoryItem(it))
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":        out,
		"limit":       params.Limit,
		"offset":      params.Offset,
		"total_count": total,
	})
}
