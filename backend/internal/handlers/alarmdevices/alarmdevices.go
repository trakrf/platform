// Package alarmdevices provides internal (session-authenticated) CRUD handlers
// for alarm_devices plus test-fire and reset actions. These are management-
// surface endpoints — NOT part of the public API (no ,public swagger tag, no
// RequireScope) because alarm devices are internal physical-layer data (TRA-903).
package alarmdevices

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/models/alarmdevice"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
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

// driver is the device transport; *shelly.Client satisfies it. Narrowed so the
// handler can be tested with a fake driver and no real device.
type driver interface {
	Set(ctx context.Context, baseURL string, switchID int, on bool) error
}

// Handler serves alarm-device CRUD + actions.
type Handler struct {
	storage   *storage.Storage
	driver    driver
	testPulse time.Duration
}

// NewHandler builds the handler. testPulse is how long a test-fire holds the
// relay on before turning it back off (pass 0 in tests to skip the wait).
func NewHandler(storage *storage.Storage, drv driver, testPulse time.Duration) *Handler {
	return &Handler{storage: storage, driver: drv, testPulse: testPulse}
}

// RegisterRoutes wires alarm-device routes onto r. Mount inside the session-auth
// (middleware.Auth) group.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/alarm-devices", h.List)
	r.Post("/api/v1/alarm-devices", h.Create)
	r.Get("/api/v1/alarm-devices/{alarm_device_id}", h.Get)
	r.Patch("/api/v1/alarm-devices/{alarm_device_id}", h.Update)
	r.Delete("/api/v1/alarm-devices/{alarm_device_id}", h.Delete)
	r.Post("/api/v1/alarm-devices/{alarm_device_id}/test", h.Test)
	r.Post("/api/v1/alarm-devices/{alarm_device_id}/reset", h.Reset)
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

// @Summary  List alarm devices
// @Tags     alarmdevices,internal
// @ID       alarmdevices.list
// @Produce  json
// @Success  200 {object} map[string]interface{}
// @Router   /api/v1/alarm-devices [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	limit, offset := parseListLimitOffset(r)
	devices, err := h.storage.ListAlarmDevices(r.Context(), orgID, limit, offset)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	total, err := h.storage.CountAlarmDevices(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       devices,
		"pagination": shared.Pagination{Page: offset/max(limit, 1) + 1, PerPage: limit, Total: total},
	})
}

// @Summary  Create an alarm device
// @Tags     alarmdevices,internal
// @ID       alarmdevices.create
// @Accept   json
// @Produce  json
// @Param    request body alarmdevice.CreateAlarmDeviceRequest true "Alarm device"
// @Success  201 {object} alarmdevice.AlarmDeviceResponse
// @Router   /api/v1/alarm-devices [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	var req alarmdevice.CreateAlarmDeviceRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	device, err := h.storage.CreateAlarmDevice(r.Context(), orgID, req)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	w.Header().Set("Location", "/api/v1/alarm-devices/"+strconv.Itoa(device.ID))
	httputil.WriteJSON(w, http.StatusCreated, alarmdevice.AlarmDeviceResponse{Data: *device})
}

// @Summary  Get an alarm device
// @Tags     alarmdevices,internal
// @ID       alarmdevices.get
// @Produce  json
// @Param    alarm_device_id path int true "Alarm device id"
// @Success  200 {object} alarmdevice.AlarmDeviceResponse
// @Router   /api/v1/alarm-devices/{alarm_device_id} [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	device, ok := h.load(w, r, orgID, reqID)
	if !ok {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, alarmdevice.AlarmDeviceResponse{Data: *device})
}

// @Summary  Update an alarm device
// @Tags     alarmdevices,internal
// @ID       alarmdevices.update
// @Accept   json
// @Produce  json
// @Param    alarm_device_id path int true "Alarm device id"
// @Param    request body alarmdevice.UpdateAlarmDeviceRequest true "Fields to update"
// @Success  200 {object} alarmdevice.AlarmDeviceResponse
// @Router   /api/v1/alarm-devices/{alarm_device_id} [patch]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("alarm_device_id", chi.URLParam(r, "alarm_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	var req alarmdevice.UpdateAlarmDeviceRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	device, err := h.storage.UpdateAlarmDevice(r.Context(), orgID, id, req)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if device == nil {
		httputil.Respond404(w, r, "alarm device not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, alarmdevice.AlarmDeviceResponse{Data: *device})
}

// @Summary  Delete an alarm device
// @Tags     alarmdevices,internal
// @ID       alarmdevices.delete
// @Param    alarm_device_id path int true "Alarm device id"
// @Success  204
// @Router   /api/v1/alarm-devices/{alarm_device_id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("alarm_device_id", chi.URLParam(r, "alarm_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	ok, err := h.storage.DeleteAlarmDevice(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if !ok {
		httputil.Respond404(w, r, "alarm device not found", reqID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary  Test-fire an alarm device (pulse on then off)
// @Tags     alarmdevices,internal
// @ID       alarmdevices.test
// @Produce  json
// @Param    alarm_device_id path int true "Alarm device id"
// @Success  200 {object} map[string]interface{}
// @Router   /api/v1/alarm-devices/{alarm_device_id}/test [post]
func (h *Handler) Test(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	device, ok := h.load(w, r, orgID, reqID)
	if !ok {
		return
	}
	ctx := r.Context()
	if err := h.driver.Set(ctx, device.BaseURL, device.SwitchID, true); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrInternal, "alarm device unreachable: "+err.Error(), reqID)
		return
	}
	if h.testPulse > 0 {
		time.Sleep(h.testPulse)
	}
	// Best-effort off; the operator can still use reset if this fails.
	_ = h.driver.Set(ctx, device.BaseURL, device.SwitchID, false)
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// @Summary  Reset (turn off) an alarm device
// @Tags     alarmdevices,internal
// @ID       alarmdevices.reset
// @Produce  json
// @Param    alarm_device_id path int true "Alarm device id"
// @Success  200 {object} map[string]interface{}
// @Router   /api/v1/alarm-devices/{alarm_device_id}/reset [post]
func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	device, ok := h.load(w, r, orgID, reqID)
	if !ok {
		return
	}
	if err := h.driver.Set(r.Context(), device.BaseURL, device.SwitchID, false); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrInternal, "alarm device unreachable: "+err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// load parses the id path param and fetches the device, writing the appropriate
// error response (400/404/500) and returning ok=false when it can't.
func (h *Handler) load(w http.ResponseWriter, r *http.Request, orgID int, reqID string) (*alarmdevice.AlarmDevice, bool) {
	id, err := httputil.ParseSurrogateID("alarm_device_id", chi.URLParam(r, "alarm_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return nil, false
	}
	device, err := h.storage.GetAlarmDeviceByID(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return nil, false
	}
	if device == nil {
		httputil.Respond404(w, r, "alarm device not found", reqID)
		return nil, false
	}
	return device, true
}
