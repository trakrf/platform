package mustering

import (
	"net/http"
	"strings"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/muster"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// maxFloorPlanImageURLLen caps the stored image_url. A data: URL can be large
// (inline base64 image) but org metadata is not a blob store — keep it modest.
const maxFloorPlanImageURLLen = 2048

// GetFloorPlan returns the org's floor plan (image + pins), or an empty plan
// when unset.
func (h *Handler) GetFloorPlan(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, _, _, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}
	fp, err := h.store.GetMusterFloorPlan(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": fp})
}

// PutFloorPlan full-replaces the org's floor plan. Validates the image URL and
// that every pin references an existing live org location with in-range
// coordinates, then persists under org metadata.
func (h *Handler) PutFloorPlan(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, _, _, ok := h.userClaims(w, r, reqID)
	if !ok {
		return
	}

	var req muster.FloorPlan
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}

	if err := validateFloorPlanShape(req); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, err.Error(), reqID)
		return
	}

	// Validate pin location ids against live org locations.
	if len(req.Pins) > 0 {
		ids := make([]int, 0, len(req.Pins))
		seen := map[int]struct{}{}
		for _, p := range req.Pins {
			if _, dup := seen[p.LocationID]; !dup {
				seen[p.LocationID] = struct{}{}
				ids = append(ids, p.LocationID)
			}
		}
		locs, err := h.store.GetLocationsByIDs(r.Context(), orgID, ids)
		if err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
			return
		}
		found := map[int]struct{}{}
		for _, l := range locs {
			found[l.ID] = struct{}{}
		}
		for _, id := range ids {
			if _, ok := found[id]; !ok {
				httputil.WriteJSONError(w, r, http.StatusUnprocessableEntity, modelerrors.ErrValidation,
					"pin references a location that does not exist for this organization", reqID)
				return
			}
		}
	}

	if err := h.store.UpdateMusterFloorPlan(r.Context(), orgID, req); err != nil {
		if err.Error() == "organization not found" {
			httputil.Respond404(w, r, "organization not found", reqID)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}

	fp, err := h.store.GetMusterFloorPlan(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": fp})
}

// validateFloorPlanShape checks the image URL and per-pin coordinate ranges.
// Location-existence is checked separately (needs storage).
func validateFloorPlanShape(fp muster.FloorPlan) error {
	url := strings.TrimSpace(fp.ImageURL)
	if url == "" {
		return errFloorPlan("image_url is required")
	}
	if len(url) > maxFloorPlanImageURLLen {
		return errFloorPlan("image_url must be at most 2048 characters")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "data:") {
		return errFloorPlan("image_url must be an http(s) or data: URL")
	}
	for _, p := range fp.Pins {
		if p.XPct < 0 || p.XPct > 100 || p.YPct < 0 || p.YPct > 100 {
			return errFloorPlan("pin x_pct and y_pct must be between 0 and 100")
		}
	}
	return nil
}

type floorPlanError string

func (e floorPlanError) Error() string { return string(e) }

func errFloorPlan(msg string) error { return floorPlanError(msg) }
