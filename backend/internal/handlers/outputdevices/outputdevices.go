// Package outputdevices provides internal (session-authenticated) CRUD handlers
// for output_devices plus test-fire and reset actions. These are management-
// surface endpoints — NOT part of the public API (no ,public swagger tag, no
// RequireScope) because output devices are internal physical-layer data (TRA-903).
package outputdevices

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
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

// actuator drives one output device on/off using its configured transport
// (http or mqtt); alarm.Dispatcher satisfies it. Narrowed so the handler can be
// tested with a fake and no real device/broker.
type actuator interface {
	Set(ctx context.Context, dev outputdevice.OutputDevice, on bool, offAfterSec int) error
}

// Handler serves output-device CRUD + actions.
type Handler struct {
	storage   *storage.Storage
	actuator  actuator
	testPulse time.Duration
}

// NewHandler builds the handler. testPulse is how long a test-fire holds the
// relay on before turning it back off (pass 0 in tests to skip the wait).
func NewHandler(storage *storage.Storage, act actuator, testPulse time.Duration) *Handler {
	return &Handler{storage: storage, actuator: act, testPulse: testPulse}
}

// RegisterRoutes wires output-device routes onto r. Mount inside the session-auth
// (middleware.Auth) group.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/v1/output-devices", h.List)
	r.Post("/api/v1/output-devices", h.Create)
	r.Get("/api/v1/output-devices/{output_device_id}", h.Get)
	r.Patch("/api/v1/output-devices/{output_device_id}", h.Update)
	r.Delete("/api/v1/output-devices/{output_device_id}", h.Delete)
	r.Post("/api/v1/output-devices/{output_device_id}/test", h.Test)
	r.Post("/api/v1/output-devices/{output_device_id}/reset", h.Reset)
}

// transportFieldsError enforces the transport-specific fields of an alarm
// device against its effective (post-merge) state. mqtt needs a command_topic
// and ignores base_url entirely; http (the default when transport is blank)
// needs a base_url that is a valid http(s) URL. Validating here rather than via
// a struct `url` tag keeps base_url from being rejected for mqtt devices, where
// it is not applicable (TRA-928). Returns "" when valid.
func transportFieldsError(transport, baseURL, commandTopic string) string {
	if transport == outputdevice.TransportMQTT {
		if commandTopic == "" {
			return "command_topic is required for mqtt transport"
		}
		return ""
	}
	if baseURL == "" {
		return "base_url is required for http transport"
	}
	if !isHTTPURL(baseURL) {
		return "base_url is not a valid value"
	}
	return ""
}

// isHTTPURL reports whether s is an absolute http(s) URL with a host. This is
// the transport-aware replacement for the former `url` struct validator.
func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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

// @Summary  List output devices
// @Tags     outputdevices,internal
// @ID       outputdevices.list
// @Produce  json
// @Success  200 {object} map[string]interface{}
// @Router   /api/v1/output-devices [get]
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	limit, offset := parseListLimitOffset(r)
	devices, err := h.storage.ListOutputDevices(r.Context(), orgID, limit, offset)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	total, err := h.storage.CountOutputDevices(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       devices,
		"pagination": shared.Pagination{Page: offset/max(limit, 1) + 1, PerPage: limit, Total: total},
	})
}

// @Summary  Create an output device
// @Tags     outputdevices,internal
// @ID       outputdevices.create
// @Accept   json
// @Produce  json
// @Param    request body outputdevice.CreateOutputDeviceRequest true "Alarm device"
// @Success  201 {object} outputdevice.OutputDeviceResponse
// @Router   /api/v1/output-devices [post]
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	var req outputdevice.CreateOutputDeviceRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	// Transport-specific validation (transport defaults to http).
	transport := req.Transport
	if transport == "" {
		transport = outputdevice.TransportHTTP
	}
	if msg := transportFieldsError(transport, req.BaseURL, deref(req.CommandTopic)); msg != "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, msg, reqID)
		return
	}
	device, err := h.storage.CreateOutputDevice(r.Context(), orgID, req)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	w.Header().Set("Location", "/api/v1/output-devices/"+strconv.Itoa(device.ID))
	httputil.WriteJSON(w, http.StatusCreated, outputdevice.OutputDeviceResponse{Data: *device})
}

// @Summary  Get an output device
// @Tags     outputdevices,internal
// @ID       outputdevices.get
// @Produce  json
// @Param    output_device_id path int true "Alarm device id"
// @Success  200 {object} outputdevice.OutputDeviceResponse
// @Router   /api/v1/output-devices/{output_device_id} [get]
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
	httputil.WriteJSON(w, http.StatusOK, outputdevice.OutputDeviceResponse{Data: *device})
}

// @Summary  Update an output device
// @Tags     outputdevices,internal
// @ID       outputdevices.update
// @Accept   json
// @Produce  json
// @Param    output_device_id path int true "Alarm device id"
// @Param    request body outputdevice.UpdateOutputDeviceRequest true "Fields to update"
// @Success  200 {object} outputdevice.OutputDeviceResponse
// @Router   /api/v1/output-devices/{output_device_id} [patch]
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("output_device_id", chi.URLParam(r, "output_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	var req outputdevice.UpdateOutputDeviceRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if err := validate.Struct(req); err != nil {
		httputil.RespondValidationError(w, r, err, reqID)
		return
	}
	// Resolve effective state for transport-aware validation: a patch may change
	// transport without resending base_url/command_topic, so merge the request
	// over the stored device before checking (TRA-928).
	existing, err := h.storage.GetOutputDeviceByID(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if existing == nil {
		httputil.Respond404(w, r, "output device not found", reqID)
		return
	}
	transport := existing.Transport
	if req.Transport != nil {
		transport = *req.Transport
	}
	baseURL := existing.BaseURL
	if req.BaseURL != nil {
		baseURL = *req.BaseURL
	}
	commandTopic := deref(existing.CommandTopic)
	if req.CommandTopic != nil {
		commandTopic = *req.CommandTopic
	}
	if msg := transportFieldsError(transport, baseURL, commandTopic); msg != "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, msg, reqID)
		return
	}
	device, err := h.storage.UpdateOutputDevice(r.Context(), orgID, id, req)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if device == nil {
		httputil.Respond404(w, r, "output device not found", reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, outputdevice.OutputDeviceResponse{Data: *device})
}

// @Summary  Delete an output device
// @Tags     outputdevices,internal
// @ID       outputdevices.delete
// @Param    output_device_id path int true "Alarm device id"
// @Success  204
// @Router   /api/v1/output-devices/{output_device_id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("output_device_id", chi.URLParam(r, "output_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	ok, err := h.storage.DeleteOutputDevice(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	if !ok {
		httputil.Respond404(w, r, "output device not found", reqID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary  Test-fire an output device (pulse on then off)
// @Tags     outputdevices,internal
// @ID       outputdevices.test
// @Produce  json
// @Param    output_device_id path int true "Alarm device id"
// @Success  200 {object} map[string]interface{}
// @Router   /api/v1/output-devices/{output_device_id}/test [post]
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
	// Backend-driven pulse for the manual test action: on, hold, off. offAfterSec
	// is 0 here (we drive the off ourselves) — the device-side toggle_after timer
	// is the geofence fire path's concern, not this synchronous diagnostic.
	if err := h.actuator.Set(ctx, *device, true, 0); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrInternal, "output device unreachable: "+err.Error(), reqID)
		return
	}
	if h.testPulse > 0 {
		time.Sleep(h.testPulse)
	}
	// Best-effort off; the operator can still use reset if this fails.
	_ = h.actuator.Set(ctx, *device, false, 0)
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// @Summary  Reset (turn off) an output device
// @Tags     outputdevices,internal
// @ID       outputdevices.reset
// @Produce  json
// @Param    output_device_id path int true "Alarm device id"
// @Success  200 {object} map[string]interface{}
// @Router   /api/v1/output-devices/{output_device_id}/reset [post]
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
	if err := h.actuator.Set(r.Context(), *device, false, 0); err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrInternal, "output device unreachable: "+err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// load parses the id path param and fetches the device, writing the appropriate
// error response (400/404/500) and returning ok=false when it can't.
func (h *Handler) load(w http.ResponseWriter, r *http.Request, orgID int, reqID string) (*outputdevice.OutputDevice, bool) {
	id, err := httputil.ParseSurrogateID("output_device_id", chi.URLParam(r, "output_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return nil, false
	}
	device, err := h.storage.GetOutputDeviceByID(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return nil, false
	}
	if device == nil {
		httputil.Respond404(w, r, "output device not found", reqID)
		return nil, false
	}
	return device, true
}
