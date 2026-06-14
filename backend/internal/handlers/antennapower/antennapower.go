// Package antennapower provides the internal (session-authenticated) endpoints
// for reading and setting a CS463's per-antenna transmit power (TRA-993). It is
// a management-surface feature, NOT part of the public API. Sets are published
// as MQTT commands to the power agent; reads return last-known values cached on
// scan_point metadata (updated by the agent's state messages).
package antennapower

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/poweragent/csl"
	"github.com/trakrf/platform/backend/internal/readerpower"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// CommandPublisher is the seam to the broker; *readercontrol.Controller
// satisfies it. A nil interface means MQTT is disabled.
type CommandPublisher interface {
	PublishPowerCommand(ctx context.Context, publishTopic string, cmd readerpower.Command) error
}

// Handler serves the antenna-power endpoints.
type Handler struct {
	storage   *storage.Storage
	publisher CommandPublisher // nil when the broker is disabled
}

// NewHandler builds a Handler. publisher may be a nil interface (sets then 503).
func NewHandler(storage *storage.Storage, publisher CommandPublisher) *Handler {
	return &Handler{storage: storage, publisher: publisher}
}

// RegisterRoutes mounts the endpoints. The read is open; the set is paid-gated
// (it mutates reader hardware config), matching scan-device mutation policy.
func (h *Handler) RegisterRoutes(r chi.Router, paidGate func(http.Handler) http.Handler) {
	r.Get("/api/v1/scan-devices/{scan_device_id}/antenna-power", h.Get)
	r.With(paidGate).Post("/api/v1/scan-devices/{scan_device_id}/antenna-power", h.Set)
}

// SetAntennaPowerRequest is the POST body. Powers maps antenna port ("1".."16")
// to dBm. Empty Powers requests a state refresh (get) without mutating.
type SetAntennaPowerRequest struct {
	Powers map[string]float64 `json:"powers"`
	Force  bool               `json:"force"`
}

// Get returns the per-antenna power view for a device.
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
	powers, err := h.storage.GetAntennaPower(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": powers})
}

// Set publishes a power command to the agent and optimistically records the
// desired values. Returns 202 (the agent confirms asynchronously via the state
// topic; poll GET for the confirmed result).
func (h *Handler) Set(w http.ResponseWriter, r *http.Request) {
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
	var req SetAntennaPowerRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}

	powers, msg := validatePowers(req.Powers)
	if msg != "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrValidation, msg, reqID)
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
	if device.PublishTopic == nil || *device.PublishTopic == "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"reader has no publish_topic; cannot route a power command", reqID)
		return
	}
	if h.publisher == nil {
		httputil.WriteJSONError(w, r, http.StatusServiceUnavailable, modelerrors.ErrInternal,
			"reader control is unavailable (broker not configured)", reqID)
		return
	}

	cmd := readerpower.Command{RequestID: uuid.NewString(), Powers: req.Powers, Force: req.Force}
	if err := h.publisher.PublishPowerCommand(r.Context(), *device.PublishTopic, cmd); err != nil {
		httputil.WriteJSONError(w, r, http.StatusServiceUnavailable, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}

	// Optimistically record desired powers (status pending) for an immediate read.
	if len(powers) > 0 {
		if err := h.storage.SetAntennaPowerDesired(r.Context(), orgID, id, powers); err != nil {
			// Non-fatal: the command was published; the cache write is best-effort.
			_ = err
		}
	}

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "request_id": cmd.RequestID})
}

// validatePowers parses the string-keyed power map into ints and bounds-checks
// it. Returns an empty message on success (powers may be empty for a get).
func validatePowers(in map[string]float64) (map[int]float64, string) {
	out := make(map[int]float64, len(in))
	for k, v := range in {
		port, err := strconv.Atoi(k)
		if err != nil || port < 1 || port > 16 {
			return nil, "antenna port must be an integer 1..16, got " + k
		}
		if v < csl.MinPowerDBm || v > csl.MaxPowerDBm {
			return nil, "transmit power must be " +
				strconv.FormatFloat(csl.MinPowerDBm, 'f', 1, 64) + ".." +
				strconv.FormatFloat(csl.MaxPowerDBm, 'f', 1, 64) + " dBm"
		}
		out[port] = v
	}
	return out, ""
}
