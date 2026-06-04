// Package scanread holds the device-agnostic parsed-read type shared between
// the MQTT ingest parser (TRA-900) and the geofence rules engine (TRA-901).
// It is a dependency-free leaf package so both the ingest and storage packages
// can import it without creating a cycle.
package scanread

import "time"

// Read is one parsed tag observation.
type Read struct {
	EPC              string
	CapturePointName string
	AntennaPort      int
	RSSI             int
	ReaderTimestamp  time.Time // informational only; server time is authoritative
}
