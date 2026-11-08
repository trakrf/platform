// Package outputdevices provides internal (session-authenticated) CRUD handlers
// for output_devices plus test-fire and reset actions. These are management-
// surface endpoints — NOT part of the public API (no ,public swagger tag, no
// RequireScope) because output devices are internal physical-layer data (TRA-903).
package outputdevices

import (
	"context"
	"math"
	"net/http"
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
// capGate is middleware.RequireCap(store)(capability.Geofence) (TRA-1025 /
// ADR 0002). Output devices are the geofence surface's alarm configuration —
// their only consumers are the geofence engine and the alarm dispatcher, and
// their tuning fields (egress/presence mode, RSSI threshold, auto-off) are the
// per-output tier of the same three-tier geofence config as
// /orgs/{id}/geofence-defaults. Gated on reads too: an org without the grant
// has no geofence surface at all.
//
// capGate precedes paidGate on the shared lines: an org cannot be past-due on a
// surface it never bought, so a lapsed org WITHOUT the grant must see
// capability_required rather than payment_required (ADR 0002 §"Backend
// enforcement").
func (h *Handler) RegisterRoutes(r chi.Router, capGate, paidGate func(http.Handler) http.Handler) {
	// TRA-947: output-device config mutations (create/update/delete) are paid;
	// GETs and the operational test/reset actions stay open.
	r.With(capGate).Get("/api/v1/output-devices", h.List)
	r.With(capGate, paidGate).Post("/api/v1/output-devices", h.Create)
	r.With(capGate).Get("/api/v1/output-devices/{output_device_id}", h.Get)
	r.With(capGate, paidGate).Patch("/api/v1/output-devices/{output_device_id}", h.Update)
	r.With(capGate, paidGate).Delete("/api/v1/output-devices/{output_device_id}", h.Delete)
	r.With(capGate).Post("/api/v1/output-devices/{output_device_id}/test", h.Test)
	r.With(capGate).Post("/api/v1/output-devices/{output_device_id}/reset", h.Reset)
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
	if deviceType == outputdevice.TypeCSLGPO {
		if msg, err := h.scanDeviceFKError(r.Context(), orgID, req.ScanDeviceID); err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
			return
		} else if msg != "" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, msg, reqID)
			return
		}
	}
	if msg, err := h.tuningError(r.Context(), orgID, outputdevice.OutputDevice{Metadata: req.Metadata}); err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	} else if msg != "" {
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
	// (post-patch) state — a patch that flips type to csl_gpo without
	// resending scan_device_id is validated against the stored value, not
	// treated as absent.
	if deviceType == outputdevice.TypeCSLGPO {
		if msg, err := h.scanDeviceFKError(r.Context(), orgID, scanDeviceID); err != nil {
			httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
			return
		} else if msg != "" {
			httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, msg, reqID)
			return
		}
	}
	// Metadata is replaced whole when sent, so the merged device carries either
	// the new object or the stored one.
	merged := *existing
	if req.Metadata != nil {
		merged.Metadata = *req.Metadata
	}
	if msg, err := h.tuningError(r.Context(), orgID, merged); err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	} else if msg != "" {
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
// @Description Drives the output off first: a pin left on after its row is gone cannot be released from the app. If the off cannot be delivered the row is kept (502) so Reset still works later; force=true deletes anyway.
// @Tags     outputdevices,internal
// @ID       outputdevices.delete
// @Param    output_device_id path int true "Alarm device id"
// @Param    force query bool false "Delete even if the output could not be driven off"
// @Success  204
// @Failure  502 {object} modelerrors.ErrorResponse
// @Router   /api/v1/output-devices/{output_device_id} [delete]
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	device, found := h.load(w, r, orgID, reqID)
	if !found {
		return
	}
	if err := h.actuator.Set(r.Context(), *device, false, 0); err != nil && r.URL.Query().Get("force") != "true" {
		httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrInternal,
			"could not turn the output off before deleting it, so it was kept: "+err.Error(), reqID)
		return
	}
	ok, err := h.storage.DeleteOutputDevice(r.Context(), orgID, device.ID)
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
	// Backend-driven pulse for the manual test action: on, hold, off. The ON also
	// arms the device-side timer for the same hold, so a lost or failed OFF
	// cannot leave the output latched on (TRA-1273): the OFF is a second
	// message, and alarm outputs fail toward off.
	offAfter := max(1, int(math.Ceil(h.testPulse.Seconds())))
	if err := h.actuator.Set(ctx, *device, true, offAfter); err != nil {
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
