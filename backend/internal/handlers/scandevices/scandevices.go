// Package scandevices provides internal (session-authenticated) CRUD handlers
// for scan_devices and their nested scan_points. These are management-surface
// endpoints — they are NOT part of the public API (no ,public swagger tag, no
// RequireScope) because scan devices/points are internal physical-layer data.
package scandevices

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanpoint"
	"github.com/trakrf/platform/backend/internal/models/shared"
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

// RegisterRoutes wires the scan-device routes (and the device-nested scan-point
// list/create) onto r. Mount inside the session-auth (middleware.Auth) group.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/scan-devices", h.List)
	r.Post("/api/v1/scan-devices", h.Create)
	r.Get("/api/v1/scan-devices/{scan_device_id}", h.Get)
	r.Patch("/api/v1/scan-devices/{scan_device_id}", h.Update)
	r.Delete("/api/v1/scan-devices/{scan_device_id}", h.Delete)
	r.Get("/api/v1/scan-devices/{scan_device_id}/scan-points", h.ListPoints)
	r.Post("/api/v1/scan-devices/{scan_device_id}/scan-points", h.CreatePoint)
}

func parseListLimitOffset(r *http.Request) (limit, offset int) {
	limit, offset = 50, 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}
	return
}

// writeConflictOrInternal maps the plain storage errors (duplicate external_key
// / publish_topic) to 409, everything else to 500.
func writeConflictOrInternal(w http.ResponseWriter, r *http.Request, err error, reqID string) {
	msg := err.Error()
	if strings.Contains(msg, "already exists") || strings.Contains(msg, "already in use") {
		httputil.WriteJSONError(w, r, http.StatusConflict, modelerrors.ErrConflict, msg, reqID)
		return
	}
	httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, msg, reqID)
}

// @Summary  List scan devices
// @Tags     scandevices,internal
// @ID       scandevices.list
// @Produce  json
// @Success  200 {object} map[string]interface{}
// @Router   /api/v1/scan-devices [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	limit, offset := parseListLimitOffset(r)
	devices, err := h.storage.ListScanDevices(r.Context(), orgID, limit, offset)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	total, err := h.storage.CountScanDevices(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       devices,
		"pagination": shared.Pagination{Page: offset/max(limit, 1) + 1, PerPage: limit, Total: total},
	})
}

// @Summary  Create a scan device
// @Tags     scandevices,internal
// @ID       scandevices.create
// @Accept   json
// @Produce  json
// @Param    request body scandevice.CreateScanDeviceRequest true "Scan device"
// @Success  201 {object} scandevice.ScanDeviceResponse
// @Router   /api/v1/scan-devices [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	var req scandevice.CreateScanDeviceRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	device, err := h.storage.CreateScanDevice(r.Context(), orgID, req)
	if err != nil {
		writeConflictOrInternal(w, r, err, reqID)
		return
	}
	w.Header().Set("Location", "/api/v1/scan-devices/"+strconv.Itoa(device.ID))
	httputil.WriteJSON(w, http.StatusCreated, scandevice.ScanDeviceResponse{Data: *device})
}

// @Summary  Get a scan device
// @Tags     scandevices,internal
// @ID       scandevices.get
// @Produce  json
// @Param    scan_device_id path int true "Scan device id"
// @Success  200 {object} scandevice.ScanDeviceResponse
// @Router   /api/v1/scan-devices/{scan_device_id} [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("scan_device_id", chi.URLParam(r, "scan_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	device, err := h.storage.GetScanDeviceByID(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if device == nil {
		httputil.Respond404(w, r, "scan device not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, scandevice.ScanDeviceResponse{Data: *device})
}

// @Summary  Update a scan device
// @Tags     scandevices,internal
// @ID       scandevices.update
// @Accept   json
// @Produce  json
// @Param    scan_device_id path int true "Scan device id"
// @Param    request body scandevice.UpdateScanDeviceRequest true "Fields to update"
// @Success  200 {object} scandevice.ScanDeviceResponse
// @Router   /api/v1/scan-devices/{scan_device_id} [patch]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("scan_device_id", chi.URLParam(r, "scan_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	var req scandevice.UpdateScanDeviceRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	device, err := h.storage.UpdateScanDevice(r.Context(), orgID, id, req)
	if err != nil {
		writeConflictOrInternal(w, r, err, reqID)
		return
	}
	if device == nil {
		httputil.Respond404(w, r, "scan device not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, scandevice.ScanDeviceResponse{Data: *device})
}

// @Summary  Delete a scan device
// @Tags     scandevices,internal
// @ID       scandevices.delete
// @Param    scan_device_id path int true "Scan device id"
// @Success  204
// @Router   /api/v1/scan-devices/{scan_device_id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("scan_device_id", chi.URLParam(r, "scan_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	ok, err := h.storage.DeleteScanDevice(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if !ok {
		httputil.Respond404(w, r, "scan device not found", reqID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary  List a device's scan points
// @Tags     scandevices,internal
// @ID       scandevices.listPoints
// @Produce  json
// @Param    scan_device_id path int true "Scan device id"
// @Success  200 {object} map[string]interface{}
// @Router   /api/v1/scan-devices/{scan_device_id}/scan-points [get]
func (h *Handler) ListPoints(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	deviceID, err := httputil.ParseSurrogateID("scan_device_id", chi.URLParam(r, "scan_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	device, err := h.storage.GetScanDeviceByID(r.Context(), orgID, deviceID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if device == nil {
		httputil.Respond404(w, r, "scan device not found", reqID)
		return
	}
	points, err := h.storage.ListScanPointsByDevice(r.Context(), orgID, deviceID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": points})
}

// @Summary  Create a scan point under a device
// @Tags     scandevices,internal
// @ID       scandevices.createPoint
// @Accept   json
// @Produce  json
// @Param    scan_device_id path int true "Scan device id"
// @Param    request body scanpoint.CreateScanPointRequest true "Scan point"
// @Success  201 {object} scanpoint.ScanPointResponse
// @Router   /api/v1/scan-devices/{scan_device_id}/scan-points [post]
func (h *Handler) CreatePoint(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	deviceID, err := httputil.ParseSurrogateID("scan_device_id", chi.URLParam(r, "scan_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	device, err := h.storage.GetScanDeviceByID(r.Context(), orgID, deviceID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if device == nil {
		httputil.Respond404(w, r, "scan device not found", reqID)
		return
	}
	var req scanpoint.CreateScanPointRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	point, err := h.storage.CreateScanPoint(r.Context(), orgID, deviceID, req)
	if err != nil {
		writeConflictOrInternal(w, r, err, reqID)
		return
	}
	w.Header().Set("Location", "/api/v1/scan-points/"+strconv.Itoa(point.ID))
	httputil.WriteJSON(w, http.StatusCreated, scanpoint.ScanPointResponse{Data: *point})
}
