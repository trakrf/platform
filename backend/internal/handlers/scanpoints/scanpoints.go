// Package scanpoints provides internal (session-authenticated) by-id CRUD for
// scan_points. Device-nested list/create live in the scandevices handler.
package scanpoints

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/scanpoint"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var validate = func() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(httputil.JSONTagNameFunc)
	httputil.RegisterCustomValidations(v)
	return v
}()

type Handler struct {
	storage *storage.Storage
}

func NewHandler(storage *storage.Storage) *Handler {
	return &Handler{storage: storage}
}

// RegisterRoutes mounts by-id scan-point routes. Mount in the session-auth group.
func (h *Handler) RegisterRoutes(r chi.Router, paidGate func(http.Handler) http.Handler) {
	// TRA-947: scan-point mutations are paid; the Get stays open.
	r.Get("/api/v1/scan-points/{scan_point_id}", h.Get)
	r.With(paidGate).Patch("/api/v1/scan-points/{scan_point_id}", h.Update)
	r.With(paidGate).Delete("/api/v1/scan-points/{scan_point_id}", h.Delete)
}

// @Summary  Get a scan point
// @Tags     scanpoints,internal
// @ID       scanpoints.get
// @Produce  json
// @Param    scan_point_id path int true "Scan point id"
// @Success  200 {object} scanpoint.ScanPointResponse
// @Router   /api/v1/scan-points/{scan_point_id} [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("scan_point_id", chi.URLParam(r, "scan_point_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	point, err := h.storage.GetScanPointByID(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if point == nil {
		httputil.Respond404(w, r, "scan point not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, scanpoint.ScanPointResponse{Data: *point})
}

// @Summary  Update a scan point
// @Tags     scanpoints,internal
// @ID       scanpoints.update
// @Accept   json
// @Produce  json
// @Param    scan_point_id path int true "Scan point id"
// @Param    request body scanpoint.UpdateScanPointRequest true "Fields to update"
// @Success  200 {object} scanpoint.ScanPointResponse
// @Router   /api/v1/scan-points/{scan_point_id} [patch]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("scan_point_id", chi.URLParam(r, "scan_point_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	var req scanpoint.UpdateScanPointRequest
	// Decode normally — location_id is a writable field, NOT read-only, so it
	// must stay in the body to populate req.LocationID. (It was previously in
	// the decoder's `drop` set, which silently stripped it and made every
	// location_id edit a no-op 200 — TRA-931.) The nulls map still distinguishes
	// an explicit `location_id: null` (detach the zone) from an omitted field.
	nulls, _, err := httputil.DecodeJSONStrictWithNullsTolerantAndPresence(r, &req, nil)
	if err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if _, ok := nulls["location_id"]; ok {
		req.ClearLocationID = true
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	point, err := h.storage.UpdateScanPoint(r.Context(), orgID, id, req)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "does not exist") {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, msg, reqID)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, msg, reqID)
		return
	}
	if point == nil {
		httputil.Respond404(w, r, "scan point not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, scanpoint.ScanPointResponse{Data: *point})
}

// @Summary  Delete a scan point
// @Tags     scanpoints,internal
// @ID       scanpoints.delete
// @Param    scan_point_id path int true "Scan point id"
// @Success  204
// @Router   /api/v1/scan-points/{scan_point_id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("scan_point_id", chi.URLParam(r, "scan_point_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	ok, err := h.storage.DeleteScanPoint(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if !ok {
		httputil.Respond404(w, r, "scan point not found", reqID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
