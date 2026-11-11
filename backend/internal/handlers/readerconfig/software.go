package readerconfig

// Reader software endpoints: read which mqtt-rpcd version a reader runs and start
// a remote update (Daemon.Update). Both use only Reader.GetCapabilities and
// Daemon.Update, which the daemon serves without touching the reader's vendor
// /API — so they work on a reader whose vendor app is wedged.

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"github.com/trakrf/platform/backend/internal/middleware"
	modelerrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/readercontrol"
	"github.com/trakrf/platform/backend/internal/readerrelease"
	"github.com/trakrf/platform/backend/internal/readerrpc"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// DaemonRPC is the seam to the reader's daemon. *readercontrol.Client satisfies it.
type DaemonRPC interface {
	GetCapabilities(ctx context.Context, base string) (readerrpc.Capabilities, error)
	UpdateDaemon(ctx context.Context, base string, p readerrpc.DaemonUpdateParams) (readerrpc.DaemonUpdateResult, error)
}

// ReleaseSource resolves a version to its signed manifest. *readerrelease.Source
// satisfies it.
type ReleaseSource interface {
	Fetch(ctx context.Context, version string) (readerrpc.DaemonUpdateParams, error)
}

// WithSoftware enables the reader-software endpoints (they 503 without it).
func (h *Handler) WithSoftware(rpc DaemonRPC, releases ReleaseSource) *Handler {
	h.daemon = rpc
	h.releases = releases
	return h
}

// UpdateSoftwareRequest is the body of POST .../reader-software/update.
type UpdateSoftwareRequest struct {
	Version string `json:"version"`
}

// deviceBase resolves the path's scan device to its RPC base topic, writing the
// error response itself when it cannot.
func (h *Handler) deviceBase(w http.ResponseWriter, r *http.Request, orgID int, reqID string) (string, bool) {
	id, err := httputil.ParseSurrogateID("scan_device_id", chi.URLParam(r, "scan_device_id"))
	if err != nil {
		httputil.RespondPathParamError(w, r, err, reqID)
		return "", false
	}
	device, err := h.storage.GetScanDeviceByID(r.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal, err.Error(), reqID)
		return "", false
	}
	if device == nil {
		httputil.Respond404(w, r, "scan device not found", reqID)
		return "", false
	}
	base := baseTopicForDevice(device)
	if base == "" {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"reader has no publish_topic; cannot route an RPC request", reqID)
		return "", false
	}
	return base, true
}

func (h *Handler) softwareUnavailable(w http.ResponseWriter, r *http.Request, reqID string) bool {
	if h.daemon == nil || h.releases == nil {
		httputil.WriteJSONError(w, r, http.StatusServiceUnavailable, modelerrors.ErrInternal,
			"reader control is unavailable (broker not configured)", reqID)
		return true
	}
	return false
}

// @Summary  Get a reader's daemon version and last update result
// @Tags     readerconfig,internal
// @ID       readerconfig.software.get
// @Produce  json
// @Param    scan_device_id path int true "Scan device id"
// @Success  200 {object} map[string]interface{}
// @Router   /api/v1/scan-devices/{scan_device_id}/reader-software [get]
func (h *Handler) GetSoftware(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	if h.softwareUnavailable(w, r, reqID) {
		return
	}
	base, ok := h.deviceBase(w, r, orgID, reqID)
	if !ok {
		return
	}
	caps, err := h.daemon.GetCapabilities(r.Context(), base)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		// Nil for a daemon that predates version reporting.
		"daemon":          caps.Daemon,
		"supports_update": slices.Contains(caps.Supports, readerrpc.MethodDaemonUpdate),
	}})
}

// @Summary  Start a remote update of a reader's daemon
// @Tags     readerconfig,internal
// @ID       readerconfig.software.update
// @Accept   json
// @Produce  json
// @Param    scan_device_id path int true "Scan device id"
// @Param    request body UpdateSoftwareRequest true "Release version"
// @Success  202 {object} map[string]interface{}
// @Router   /api/v1/scan-devices/{scan_device_id}/reader-software/update [post]
func (h *Handler) UpdateSoftware(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := middleware.GetRequestOrgID(r)
	if err != nil {
		httputil.RespondMissingOrgContext(w, r, reqID)
		return
	}
	if h.softwareUnavailable(w, r, reqID) {
		return
	}
	var req UpdateSoftwareRequest
	if err := httputil.DecodeJSONStrict(r, &req); err != nil {
		httputil.RespondDecodeError(w, r, err, reqID)
		return
	}
	if !readerrelease.ValidVersion(req.Version) {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest, "version is required and must be a release version", reqID)
		return
	}
	base, ok := h.deviceBase(w, r, orgID, reqID)
	if !ok {
		return
	}
	manifest, err := h.releases.Fetch(r.Context(), req.Version)
	switch {
	case errors.Is(err, readerrelease.ErrNotFound):
		httputil.Respond404(w, r, "no mqtt-rpcd release "+req.Version, reqID)
		return
	case err != nil:
		httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrInternal, err.Error(), reqID)
		return
	}
	res, err := h.daemon.UpdateDaemon(r.Context(), base, manifest)
	if err != nil {
		var re *readerrpc.UpdateRefusedError
		switch {
		case errors.As(err, &re):
			// The reader checked the request and changed nothing; detail is its reason.
			httputil.WriteJSONError(w, r, http.StatusUnprocessableEntity, modelerrors.ErrValidation, re.Reason, reqID)
		case errors.Is(err, readercontrol.ErrNoReply):
			// Sent, but the reply was lost: the update may be running. 504, not
			// 502, so the app watches the reader instead of saying "not started".
			httputil.WriteJSONError(w, r, http.StatusGatewayTimeout, modelerrors.ErrInternal, err.Error(), reqID)
		default:
			httputil.WriteJSONError(w, r, http.StatusBadGateway, modelerrors.ErrInternal, err.Error(), reqID)
		}
		return
	}
	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{"data": res})
}
