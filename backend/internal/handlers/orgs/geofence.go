package orgs

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/geofence"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// GeofenceDefaultsView is the GET/PATCH payload (TRA-955): the stored org-tier
// overrides plus the system/code-tier values, so the UI can render unset fields
// as "blank = system default (X)".
type GeofenceDefaultsView struct {
	Defaults       organization.GeofenceDefaults `json:"defaults"`
	SystemDefaults geofence.Tuning               `json:"system_defaults"`
}

// validateGeofenceDefaults checks the provided (non-nil) org-default fields.
// nil fields mean "unset" (fall back to the system tier) and are always allowed.
func validateGeofenceDefaults(d organization.GeofenceDefaults) error {
	if d.Mode != nil && *d.Mode != outputdevice.ModeEgress && *d.Mode != outputdevice.ModePresence {
		return fmt.Errorf("mode must be %q or %q", outputdevice.ModeEgress, outputdevice.ModePresence)
	}
	if d.RSSIThreshold != nil && (*d.RSSIThreshold < -120 || *d.RSSIThreshold > 0) {
		return fmt.Errorf("rssi_threshold must be between -120 and 0 dBm")
	}
	if d.AgeOutSeconds != nil && *d.AgeOutSeconds < 1 {
		return fmt.Errorf("age_out_seconds must be >= 1")
	}
	if d.AutoOffSeconds != nil && *d.AutoOffSeconds < 0 {
		return fmt.Errorf("auto_off_seconds must be >= 0")
	}
	return nil
}

func geofenceDefaultsView(d organization.GeofenceDefaults) GeofenceDefaultsView {
	return GeofenceDefaultsView{
		Defaults:       d,
		SystemDefaults: geofence.SystemTuning(geofence.DefaultConfig()),
	}
}

// @Summary Get an organization's geofence tuning defaults
// @Description Internal-only. Returns the org-tier overrides plus the system/code-tier defaults for placeholder display.
// @Tags orgs,internal
// @ID orgs.geofence-defaults.get
// @Accept json
// @Produce json
// @Param id path int true "Organization id" minimum(1) format(int64)
// @Success 200 {object} map[string]any "data: GeofenceDefaultsView"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security SessionAuth
// @Router /api/v1/orgs/{id}/geofence-defaults [get]
// GetGeofenceDefaults returns the org-tier geofence tuning overrides.
func (h *Handler) GetGeofenceDefaults(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	d, err := h.storage.GetOrgGeofenceDefaults(r.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to get geofence defaults", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": geofenceDefaultsView(d)})
}

// @Summary Replace an organization's geofence tuning defaults
// @Description Internal-only. Full-replace: the geofence_defaults object is rebuilt from the provided non-null fields; omitted/null fields fall back to the system default.
// @Tags orgs,internal
// @ID orgs.geofence-defaults.patch
// @Accept json
// @Produce json
// @Param id path int true "Organization id" minimum(1) format(int64)
// @Param request body organization.GeofenceDefaults true "Org geofence defaults"
// @Success 200 {object} map[string]any "data: GeofenceDefaultsView"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 415 {object} modelerrors.ErrorResponse "unsupported_media_type"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security SessionAuth
// @Router /api/v1/orgs/{id}/geofence-defaults [patch]
// PatchGeofenceDefaults replaces the org-tier geofence tuning overrides.
func (h *Handler) PatchGeofenceDefaults(w http.ResponseWriter, r *http.Request) {
	id, err := httputil.ParseSurrogateID("id", chi.URLParam(r, "id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, middleware.GetRequestID(r.Context()))
		return
	}

	var req organization.GeofenceDefaults
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := validateGeofenceDefaults(req); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation,
			err.Error(), middleware.GetRequestID(r.Context()))
		return
	}

	if err := h.storage.UpdateOrgGeofenceDefaults(r.Context(), id, req); err != nil {
		if err.Error() == "organization not found" {
			httputil.Respond404(w, r, "Organization not found", middleware.GetRequestID(r.Context()))
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to update geofence defaults", middleware.GetRequestID(r.Context()))
		return
	}

	d, err := h.storage.GetOrgGeofenceDefaults(r.Context(), id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to read back geofence defaults", middleware.GetRequestID(r.Context()))
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": geofenceDefaultsView(d)})
}
