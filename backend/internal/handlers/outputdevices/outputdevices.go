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
func (h *Handler) RegisterRoutes(r chi.Router, paidGate func(http.Handler) http.Handler) {
	// TRA-947: output-device config mutations (create/update/delete) are paid;
	// GETs and the operational test/reset actions stay open.
	r.Get("/api/v1/output-devices", h.List)
	r.With(paidGate).Post("/api/v1/output-devices", h.Create)
	r.Get("/api/v1/output-devices/{output_device_id}", h.Get)
	r.With(paidGate).Patch("/api/v1/output-devices/{output_device_id}", h.Update)
	r.With(paidGate).Delete("/api/v1/output-devices/{output_device_id}", h.Delete)
	r.Post("/api/v1/output-devices/{output_device_id}/test", h.Test)
	r.Post("/api/v1/output-devices/{output_device_id}/reset", h.Reset)
}

// deviceFieldsError enforces the type- and transport-specific fields of an
// output device against its effective (post-merge) state. Validating here
// rather than via struct tags keeps a field from being rejected for a device
// type where it does not apply (TRA-928, extended for TRA-1028).
//
//   - csl_cs463_gpo: mqtt only; the reader is addressed by scan_device_id (an
//     FK, checked against storage separately — see Create/Update), NOT by
//     command_topic, which this pure function does not require for GPO; the
//     1-based GPO port lives in switch_id and must be 1-4.
//   - shelly_gen4 over mqtt: needs a command_topic, ignores base_url.
//   - shelly_gen4 over http: needs a base_url that is a valid http(s) URL.
//
// Returns "" when valid.
func deviceFieldsError(deviceType, transport, baseURL, commandTopic string, switchID int) string {
	if deviceType == outputdevice.TypeCS463GPO {
		if transport != outputdevice.TransportMQTT {
			return "csl_cs463_gpo requires mqtt transport"
		}
		if switchID < 1 || switchID > 4 {
			return "switch_id is the GPO port and must be between 1 and 4 for csl_cs463_gpo"
		}
		return ""
	}
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

// scanDeviceFKError checks the storage-backed half of GPO validation that
// deviceFieldsError cannot do (it is pure, with no storage access): a
// csl_cs463_gpo device's effective scan_device_id must be present, must
// reference a live reader in orgID, that reader must be a csl_cs463 (the only
// type with a CS463 daemon listening for Gpo.Set), and that reader must have
// a non-empty publish_topic (the alarm dispatcher derives the reader's RPC
// base topic from it at fire time; with none, the base topic is "" and the
// dispatcher fail-closed refuses to fire — silently, unless caught here).
// Returns a non-"" message for the caller to surface as 400, or a non-nil
// error for an unexpected storage failure (500). Both are "" only when
// scanDeviceID resolves to a fireable reader.
func (h *Handler) scanDeviceFKError(ctx context.Context, orgID int, scanDeviceID *int) (msg string, err error) {
	if scanDeviceID == nil {
		return "scan_device_id is required for csl_cs463_gpo", nil
	}
	check, err := h.storage.CheckGPOReader(ctx, orgID, *scanDeviceID)
	if err != nil {
		return "", err
	}
	if !check.Found {
		return "scan_device_id must reference one of your readers", nil
	}
	if !check.IsCS463 {
		return "scan_device_id must reference a csl_cs463 reader", nil
	}
	if !check.HasPublishTopic {
		return "scan_device_id must reference a reader with a publish_topic configured", nil
	}
	return "", nil
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
	// Type-/transport-specific validation (both default server-side).
	transport := req.Transport
	if transport == "" {
		transport = outputdevice.TransportHTTP
	}
	deviceType := req.Type
	if deviceType == "" {
		deviceType = outputdevice.TypeShellyGen4
	}
	switchID := 0
	if req.SwitchID != nil {
		switchID = *req.SwitchID
	}
	if msg := deviceFieldsError(deviceType, transport, req.BaseURL, deref(req.CommandTopic), switchID); msg != "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, msg, reqID)
		return
	}
	// TRA-1028: a GPO device is addressed solely by its reader FK — require it
	// and confirm it resolves to a live reader in this org, closing the
	// cross-org actuation hole the free-text command_topic left open.
	if deviceType == outputdevice.TypeCS463GPO {
		if msg, err := h.scanDeviceFKError(r.Context(), orgID, req.ScanDeviceID); err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
			return
		} else if msg != "" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, msg, reqID)
			return
		}
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
	// location_id is writable, so it stays out of the decoder's drop set; the
	// nulls map lets us tell an explicit `location_id: null` (detach the location)
	// from an omitted field (leave it unchanged) — TRA-931 / TRA-940.
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
	// Resolve effective state for type-/transport-aware validation: a patch may
	// change type or transport without resending switch_id/base_url/command_topic,
	// so merge the request over the stored device before checking (TRA-928,
	// extended for TRA-1028).
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
	deviceType := existing.Type
	if req.Type != nil {
		deviceType = *req.Type
	}
	switchID := existing.SwitchID
	if req.SwitchID != nil {
		switchID = *req.SwitchID
	}
	scanDeviceID := existing.ScanDeviceID
	if req.ScanDeviceID != nil {
		scanDeviceID = req.ScanDeviceID
	}
	if msg := deviceFieldsError(deviceType, transport, baseURL, commandTopic, switchID); msg != "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, msg, reqID)
		return
	}
	// TRA-1028: same FK requirement as Create, evaluated against the merged
	// (post-patch) state — a patch that flips type to csl_cs463_gpo without
	// resending scan_device_id is validated against the stored value, not
	// treated as absent.
	if deviceType == outputdevice.TypeCS463GPO {
		if msg, err := h.scanDeviceFKError(r.Context(), orgID, scanDeviceID); err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
			return
		} else if msg != "" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, msg, reqID)
			return
		}
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
