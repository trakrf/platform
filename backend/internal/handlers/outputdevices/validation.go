package outputdevices

import (
	"context"
	"net/url"

	"github.com/trakrf/platform/backend/internal/geofence"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

// Write-path validation for output devices: the checks Create and Update run
// against a device's effective (post-merge) state before storage sees it.

// deviceFieldsError enforces the type- and transport-specific fields of an
// output device against its effective (post-merge) state. Validating here
// rather than via struct tags keeps a field from being rejected for a device
// type where it does not apply (TRA-928, extended for TRA-1028).
//
//   - csl_gpo: mqtt only; the reader is addressed by scan_device_id (an
//     FK, checked against storage separately — see Create/Update), NOT by
//     command_topic, which this pure function does not require for GPO; the
//     1-based GPO port lives in switch_id and must be 1-4.
//   - shelly_gen4 over mqtt: needs a command_topic, ignores base_url.
//   - shelly_gen4 over http: needs a base_url that is a valid http(s) URL.
//
// Returns "" when valid.
func deviceFieldsError(deviceType, transport, baseURL, commandTopic string, switchID int) string {
	if deviceType == outputdevice.TypeCSLGPO {
		if transport != outputdevice.TransportMQTT {
			return "csl_gpo requires mqtt transport"
		}
		if switchID < 1 || switchID > 4 {
			return "switch_id is the GPO port and must be between 1 and 4 for csl_gpo"
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
// csl_gpo device's effective scan_device_id must be present, must
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
		return "scan_device_id is required for csl_gpo", nil
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

// tuningError checks dev's effective tuning against the org defaults
// (geofence.OutputTuningError): an egress output with no resolved auto-off has
// no OFF edge and latches on forever (TRA-1273), and no on-time may exceed the
// reader's ceiling. Same (msg, err) contract as scanDeviceFKError.
func (h *Handler) tuningError(ctx context.Context, orgID int, dev outputdevice.OutputDevice) (msg string, err error) {
	org, err := h.storage.GetOrgGeofenceDefaults(ctx, orgID)
	if err != nil {
		return "", err
	}
	return geofence.OutputTuningError(org, dev), nil
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
