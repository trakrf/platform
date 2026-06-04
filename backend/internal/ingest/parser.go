// Package ingest contains the in-backend MQTT subscriber that replaces the
// Redpanda Connect ingester and the process_tag_scans PG trigger (TRA-900).
package ingest

import (
	"errors"
	"time"

	"github.com/trakrf/platform/backend/internal/models/scandevice"
)

// ErrUnsupportedDevice is returned by Parse when a device type has no parser
// yet (GL-S10 / ESP32 / CS108 are deferred to their own tickets).
var ErrUnsupportedDevice = errors.New("ingest: unsupported device type")

// Read is one parsed tag observation, device-agnostic. Shared with the TRA-901
// geofence engine so there is a single in-Go parser per device type.
type Read struct {
	EPC              string
	CapturePointName string
	AntennaPort      int
	RSSI             int
	ReaderTimestamp  time.Time // informational only; server time is authoritative
}

// Parse dispatches a raw MQTT payload to the parser for the registered device
// type. It never panics on bad input — malformed payloads return an error.
func Parse(deviceType string, payload []byte) ([]Read, error) {
	switch deviceType {
	case scandevice.DeviceTypeCS463:
		return parseCS463(payload)
	default:
		return nil, ErrUnsupportedDevice
	}
}
