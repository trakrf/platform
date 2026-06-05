// Package ingest contains the in-backend MQTT subscriber that replaces the
// Redpanda Connect ingester and the process_tag_scans PG trigger (TRA-900).
package ingest

import (
	"errors"

	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// ErrUnsupportedDevice is returned by Parse when a device type has no parser
// yet (ESP32 / CS108 are deferred to their own tickets).
var ErrUnsupportedDevice = errors.New("ingest: unsupported device type")

// Parse dispatches a raw MQTT payload to the parser for the registered device
// type. It never panics on bad input — malformed payloads return an error.
func Parse(deviceType string, payload []byte) ([]scanread.Read, error) {
	switch deviceType {
	case scandevice.DeviceTypeCS463:
		return parseCS463(payload)
	case scandevice.DeviceTypeGLS10:
		return parseGLS10(payload)
	default:
		return nil, ErrUnsupportedDevice
	}
}
