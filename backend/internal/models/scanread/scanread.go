// Package scanread holds the device-agnostic parsed-read type shared between
// the MQTT ingest parser (TRA-900) and the geofence rules engine (TRA-901).
// It is a dependency-free leaf package so both the ingest and storage packages
// can import it without creating a cycle.
package scanread

import "time"

// Read is one parsed tag observation. The scan_point it belongs to is resolved
// downstream (storage.PersistReads) from the device the topic routed to plus
// AntennaPort — there is no device-reported capture-point string anymore
// (TRA-956). AntennaPort defaults to 1 for single-antenna devices.
type Read struct {
	EPC             string
	AntennaPort     int
	RSSI            int
	ReaderTimestamp time.Time // informational only; server time is authoritative
}
