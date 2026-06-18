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
// heard in a scan window; each dev_list entry becomes one read.
type glS10Payload struct {
	DevBLEMac string       `json:"dev_ble_mac"`
	DevList   []glS10Entry `json:"dev_list"`
}

type glS10Entry struct {
	MAC  string `json:"mac"`
	RSSI int    `json:"rssi"` // already dBm; absent -> 0
	TS   int64  `json:"ts"`   // milliseconds since epoch (CS463 uses microseconds)
	AD   string `json:"ad"`   // raw advertisement hex; decoded for the Live Reads filter (TRA-926)
}

// parseGLS10 decodes a GL-S10 BLE gateway message into one read per detected
// advertisement. The topic routes the message to the device (resolve_scan_topic,
// TRA-900); a BLE gateway is a single capture point, so every read carries
// antenna port 1 and resolves to the device's antenna-1 scan_point downstream
// (storage.PersistReads, TRA-956). The only remaining provisioning contract is
// asset membership: the BLE MAC must be registered as a tag value linked to an
// asset. Membership is tag-class agnostic (TRA-927), so the natural type='ble'
// registration resolves the same as an rfid EPC.
func parseGLS10(payload []byte) ([]scanread.Read, error) {
	var p glS10Payload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("gl_s10: unmarshal payload: %w", err)
	}
	// The MAC is the tag identity: it is matched case-insensitively downstream
	// (TRA-944) but registered uppercase, so normalize the read EPC to uppercase
	// to tolerate lowercase wire variants.
	reads := make([]scanread.Read, 0, len(p.DevList))
	for _, e := range p.DevList {
		reads = append(reads, scanread.Read{
			EPC:             strings.ToUpper(e.MAC),
			AntennaPort:     1,
			RSSI:            e.RSSI,
			ReaderTimestamp: time.UnixMilli(e.TS).UTC(),
			// Classify the advertisement for the Live-Reads noise filter (TRA-926).
			// Never affects membership/asset_scans, which ignore BLE.
			BLE: decodeBLEAdvert(e.AD),
		})
	}
	return reads, nil
}
