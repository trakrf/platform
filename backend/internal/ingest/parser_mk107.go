package ingest

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// mk107ScanReport is the msg_id that carries a batch of heard advertisements.
// Other frames (e.g. 3003 status/heartbeat) carry an object `data` and are
// ignored without error.
const mk107ScanReport = 3004

// mk107Payload is the MOKO MK107 Pro BLE gateway message envelope (verified
// against live preview traffic 2026-06-08). On a scan report the gateway lists
// every BLE advertisement it heard in a window; each data[] entry becomes one
// read. Unlike the GL-S10 (raw ad hex), the MK107 pre-decodes iBeacons, but
// every entry is read the same way — by its value.mac. Per-entry filtering
// (iBeacon/Eddystone only) is intentionally deferred — see TRA-926.
type mk107Payload struct {
	MsgID      int             `json:"msg_id"`
	DeviceInfo mk107DeviceInfo `json:"device_info"`
	// Data is an array on scan reports (3004) and an object on status frames
	// (3003); decode it lazily so a status frame doesn't fail the envelope.
	Data json.RawMessage `json:"data"`
}

type mk107DeviceInfo struct {
	MAC string `json:"mac"`
}

type mk107Entry struct {
	Value mk107Value `json:"value"`
}

type mk107Value struct {
	MAC  string `json:"mac"`
	RSSI int    `json:"rssi"` // already dBm; absent -> 0
}

// parseMK107 decodes a MK107 scan report into one read per heard advertisement.
// The same two provisioning contracts as the GL-S10 make those reads resolve to
// asset_scans downstream (storage.PersistReads):
//   - The device is registered with external_key == the gateway MAC, so the
//     synthesized capture point {mac}-1 matches the scan_point that
//     CreateScanDevice auto-provisions as {external_key}-1.
//   - The heard MAC is registered as a tag value linked to an asset.
//     Membership is tag-class agnostic (TRA-927), so a type='ble' registration
//     resolves the same as an rfid EPC.
//
// The MK107 timestamp is non-standard ("2026-6-8&16:12:51+00") and intentionally
// dropped; server receivedAt is authoritative, consistent with existing ingest,
// so ReaderTimestamp is left zero.
func parseMK107(payload []byte) ([]scanread.Read, error) {
	var p mk107Payload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("mk107: unmarshal payload: %w", err)
	}
	// Only scan reports carry the data[] array; ignore status/heartbeat and any
	// other frame without error (and without a read).
	if p.MsgID != mk107ScanReport {
		return nil, nil
	}
	var entries []mk107Entry
	if err := json.Unmarshal(p.Data, &entries); err != nil {
		return nil, fmt.Errorf("mk107: unmarshal data array: %w", err)
	}
	// The gateway is a single capture point and sends no per-read capture point,
	// so synthesize the registered scan_point external_key as {gateway mac}-1.
	// The MAC is matched case-sensitively downstream against uppercase-registered
	// values (tags.value, scan_points.external_key); normalize to uppercase to
	// tolerate lowercase wire variants.
	capturePoint := strings.ToUpper(p.DeviceInfo.MAC) + "-1"
	reads := make([]scanread.Read, 0, len(entries))
	for _, e := range entries {
		reads = append(reads, scanread.Read{
			EPC:              strings.ToUpper(e.Value.MAC),
			CapturePointName: capturePoint,
			AntennaPort:      1,
			RSSI:             e.Value.RSSI,
		})
	}
	return reads, nil
}
