// Package readerconfig provides the internal (session-authenticated) endpoints
// for reading and setting a reader's live configuration over the MQTT JSON-RPC
// control contract (TRA-993). It is a management-surface feature, NOT part of
// the public API (no ,public swagger tag, no RequireScope): readers are internal
// physical-layer devices.
//
// Reads (GET) call the reader synchronously via the RPC client and return its
// capabilities + current config. Sets (PATCH) push a (partial) config and return
// the reader's SetConfigResult. When the broker is not configured the RPC client
// is nil and these endpoints report a clear 503.
package readerconfig

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/readerrpc"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// Transmit-power bounds (dBm) accepted on a SetOperProfile — the CS463's operational
// range (Indy RS2000: 10.0–31.5 dBm in 0.5 dB steps). Below ~10 dBm the read zone
// is a few inches (meaningless); 31.5 is the module max. The daemon's capabilities
// report the same range; the reader also enforces its own cap.
const (
	minTxPowerDBm = 10.0
	maxTxPowerDBm = 31.5
)

// RPCClient is the seam to the reader RPC transport. *readercontrol.Client
// satisfies it; a nil interface means reader control is disabled (endpoints 503).
type RPCClient interface {
	GetCapabilities(ctx context.Context, base string) (readerrpc.Capabilities, error)
	GetOperProfile(ctx context.Context, base string, force bool) (readerrpc.ReaderConfig, error)
	SetOperProfile(ctx context.Context, base string, cfg readerrpc.ReaderConfig, force bool) (readerrpc.SetConfigResult, error)
}

// Handler serves the reader-config endpoints.
type Handler struct {
	storage *storage.Storage
	rpc     RPCClient // nil when the broker is disabled
}

// NewHandler builds a Handler. rpc may be a nil interface (endpoints then 503).
func NewHandler(storage *storage.Storage, rpc RPCClient) *Handler {
	return &Handler{storage: storage, rpc: rpc}
}

// RegisterRoutes mounts the endpoints inside the session-auth group. The read is
// open; the set is paid-gated (it mutates reader hardware config), matching the
// scan-device mutation policy.
func (h *Handler) RegisterRoutes(r chi.Router, paidGate func(http.Handler) http.Handler) {
	r.Get("/api/v1/scan-devices/{scan_device_id}/reader-config", h.Get)
	r.With(paidGate).Patch("/api/v1/scan-devices/{scan_device_id}/reader-config", h.Set)
}

// baseTopicForDevice derives a reader's RPC base topic from its publish_topic by
// stripping a trailing "/reads" segment (e.g. "trakrf.id/cs463-212/reads" ->
// "trakrf.id/cs463-212"). The RPC request topic is then <base>/rpc. Returns ""
// when the device has no publish_topic (the caller maps that to 400).
func baseTopicForDevice(d *scandevice.ScanDevice) string {
	if d == nil || d.PublishTopic == nil || *d.PublishTopic == "" {
		return ""
	}
	return strings.TrimSuffix(*d.PublishTopic, "/reads")
}

// validateTxPower bounds-checks the power of every enabled antenna in a config.
// Returns an empty message on success.
func validateTxPower(cfg readerrpc.ReaderConfig) string {
	for _, ac := range cfg.Antennas {
		if ac.Enabled && (ac.PowerDBm < minTxPowerDBm || ac.PowerDBm > maxTxPowerDBm) {
			return "power_dbm must be between 10 and 31.5 dBm for enabled antennas"
		}
	}
	return ""
}

func wantForce(r *http.Request) bool { return r.URL.Query().Get("force") == "true" }

func enabledPorts(cfg readerrpc.ReaderConfig) []int {
	out := []int{}
	for _, ac := range cfg.Antennas {
		if ac.Enabled {
			out = append(out, ac.Antenna)
		}
	}
	return out
}

// respondBusy maps a reader single-session lock to a typed 409.
func respondBusy(w http.ResponseWriter, be *readerrpc.BusyError) {
	httputil.WriteJSON(w, http.StatusConflict, map[string]any{
		"error":   "reader_busy",
		"held_by": be.HeldBy,
	})
}

// @Summary  Get a reader's live configuration
// @Tags     readerconfig,internal
// @ID       readerconfig.get
// @Produce  json
// @Param    scan_device_id path int true "Scan device id"
// @Success  200 {object} map[string]interface{}
// @Router   /api/v1/scan-devices/{scan_device_id}/reader-config [get]
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	if h.rpc == nil {
		httputil.WriteJSONError(w, r, http.StatusServiceUnavailable, modelerrors.ErrInternal,
			"reader control is unavailable (broker not configured)", reqID)
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
	base := baseTopicForDevice(device)
	if base == "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"reader has no publish_topic; cannot route an RPC request", reqID)
		return
	}

	caps, err := h.rpc.GetCapabilities(r.Context(), base)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	cfg, err := h.rpc.GetOperProfile(r.Context(), base, wantForce(r))
	if err != nil {
		var be *readerrpc.BusyError
		if errors.As(err, &be) {
			respondBusy(w, be)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	// Best-effort: mirror the reader's live enablement onto scan_points.is_active
	// (the reader is the source of truth; this keeps the denormalized copy honest).
	_ = h.storage.SetScanPointActiveByPorts(r.Context(), orgID, id, enabledPorts(cfg))

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"capabilities": caps,
		"config":       cfg,
	}})
}

// @Summary  Set a reader's configuration
// @Tags     readerconfig,internal
// @ID       readerconfig.set
// @Accept   json
// @Produce  json
// @Param    scan_device_id path int true "Scan device id"
// @Param    request body readerrpc.ReaderConfig true "Reader configuration"
// @Success  202 {object} map[string]interface{}
// @Router   /api/v1/scan-devices/{scan_device_id}/reader-config [patch]
func (h *Handler) Set(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	if h.rpc == nil {
		httputil.WriteJSONError(w, r, http.StatusServiceUnavailable, modelerrors.ErrInternal,
			"reader control is unavailable (broker not configured)", reqID)
		return
	}
	id, err := httputil.ParseSurrogateID("scan_device_id", chi.URLParam(r, "scan_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return
	}
	var cfg readerrpc.ReaderConfig
	if err := httputil.DecodeJSONStrict(r, &cfg); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if msg := validateTxPower(cfg); msg != "" {
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
	base := baseTopicForDevice(device)
	if base == "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"reader has no publish_topic; cannot route an RPC request", reqID)
		return
	}

	res, err := h.rpc.SetOperProfile(r.Context(), base, cfg, wantForce(r))
	if err != nil {
		var be *readerrpc.BusyError
		if errors.As(err, &be) {
			respondBusy(w, be)
			return
		}
		httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	// Only reconcile scan_points when the request actually carried enablement —
	// a read-timing-only PATCH (dwell/dedup/antDiff, no antennas) must NOT touch
	// is_active (enabledPorts would be empty → it would disable every point).
	if len(cfg.Antennas) > 0 {
		_ = h.storage.SetScanPointActiveByPorts(r.Context(), orgID, id, enabledPorts(cfg))
	}
	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{"data": res})
}
