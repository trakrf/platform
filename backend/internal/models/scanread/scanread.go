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
	// BLE is the decoded BLE advertisement classification for this read, set by
	// BLE-gateway parsers (GL-S10, MK107). It is nil for RFID reads (CS463) and
	// is consumed ONLY by the Live Reads noise filter (TRA-926) — membership,
	// asset_scans, and geofence never read it.
	BLE *BLEAdvert
}

// BLE advertisement type discriminators (TRA-926). Eddystone is a future seam;
// any non-iBeacon advertisement classifies as BLETypeUnknown for now.
const (
	BLETypeIBeacon = "ibeacon"
	BLETypeUnknown = "unknown"
)

// BLEAdvert is a read's decoded BLE advertisement. Type is the discriminator the
// Live Reads filter keys on; UUID/Major/Minor are populated only when
// Type == BLETypeIBeacon (kept for debugging/future use, not surfaced in the UI).
type BLEAdvert struct {
	Type  string
	UUID  string // iBeacon proximity UUID, uppercase hex, no dashes
	Major uint16
	Minor uint16
}
