package ingest

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// glS10Payload is the GL-S10 BLE gateway message shape (verified against live
// preview traffic 2026-06-05). The gateway reports every BLE advertisement it
// heard in a scan window; each dev_list entry becomes one read. EPC filtering
// (e.g. iBeacon/Eddystone only) is intentionally deferred — see TRA-925.
type glS10Payload struct {
	DevBLEMac string       `json:"dev_ble_mac"`
	DevList   []glS10Entry `json:"dev_list"`
}

type glS10Entry struct {
	MAC  string `json:"mac"`
	RSSI int    `json:"rssi"` // already dBm; absent -> 0
	TS   int64  `json:"ts"`   // milliseconds since epoch (CS463 uses microseconds)
}

// parseGLS10 decodes a GL-S10 BLE gateway message into one read per detected
// advertisement. Two provisioning contracts make those reads resolve to
// asset_scans downstream (storage.PersistReads); both are pinned by the
// integration test and hold for the live preview registration:
//   - The device must be registered with external_key == dev_ble_mac, so the
//     synthesized capture point {dev_ble_mac}-1 matches the scan_point that
//     CreateScanDevice auto-provisions as {external_key}-1.
//   - The BLE MAC must be registered as a tag value linked to an asset.
//     Membership is tag-class agnostic (TRA-927), so the natural type='ble'
//     registration resolves the same as an rfid EPC.
func parseGLS10(payload []byte) ([]scanread.Read, error) {
	var p glS10Payload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("gl_s10: unmarshal payload: %w", err)
	}
	// A BLE gateway is a single capture point and sends no per-read capture
	// point, so we synthesize the registered scan_point external_key as
	// {dev_ble_mac}-1 (matches the preview registration C4DEE229A176-1). The MAC
	// is also the tag identity: it is matched case-sensitively downstream against
	// uppercase-registered values (tags.value, scan_points.external_key), so
	// normalize both to uppercase to tolerate lowercase wire variants.
	capturePoint := strings.ToUpper(p.DevBLEMac) + "-1"
	reads := make([]scanread.Read, 0, len(p.DevList))
	for _, e := range p.DevList {
		reads = append(reads, scanread.Read{
			EPC:              strings.ToUpper(e.MAC),
			CapturePointName: capturePoint,
			AntennaPort:      1,
			RSSI:             e.RSSI,
			ReaderTimestamp:  time.UnixMilli(e.TS).UTC(),
		})
	}
	return reads, nil
}
