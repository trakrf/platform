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

// mk107TypeIBeacon is the per-entry `type` discriminator the gateway sets when it
// pre-decodes an advertisement as an iBeacon (0 = Unknown raw advertisement).
const mk107TypeIBeacon = 1

// mk107Payload is the MOKO MK107 Pro BLE gateway message envelope (verified
// against live preview traffic 2026-06-08). On a scan report the gateway lists
// every BLE advertisement it heard in a window; each data[] entry becomes one
// read. Unlike the GL-S10 (raw ad hex), the MK107 pre-decodes iBeacons with
// type/uuid/major/minor fields. Per-entry classification (iBeacon → BLEAdvert;
// else → BLETypeUnknown) feeds the Live-Reads noise filter (TRA-926).
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
	Type  int        `json:"type"` // 0 = Unknown, 1 = iBeacon (TRA-926)
	Value mk107Value `json:"value"`
}

type mk107Value struct {
	MAC   string `json:"mac"`
	RSSI  int    `json:"rssi"`  // already dBm; absent -> 0
	UUID  string `json:"uuid"`  // iBeacon only (type 1)
	Major uint16 `json:"major"` // iBeacon only
	Minor uint16 `json:"minor"` // iBeacon only
}

// parseMK107 decodes a MK107 scan report into one read per heard advertisement.
// Like the GL-S10, the topic routes the message to its device (TRA-900) and the
// gateway is a single capture point, so every read carries antenna port 1 and
// resolves to the device's antenna-1 scan_point downstream (storage.PersistReads,
// TRA-956). The only remaining provisioning contract is asset membership: the
// heard MAC must be registered as a tag value linked to an asset. Membership is
// tag-class agnostic (TRA-927), so a type='ble' registration resolves the same
// as an rfid EPC.
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
	// The MAC is matched case-insensitively downstream (TRA-944) but registered
	// uppercase; normalize the read EPC to uppercase to tolerate lowercase wire
	// variants.
	reads := make([]scanread.Read, 0, len(entries))
	for _, e := range entries {
		// Classify for the Live-Reads noise filter (TRA-926). The MK107 pre-decodes
		// iBeacons (type 1) with uuid/major/minor; everything else is unknown.
		ble := &scanread.BLEAdvert{Type: scanread.BLETypeUnknown}
		if e.Type == mk107TypeIBeacon {
			ble = &scanread.BLEAdvert{
				Type:  scanread.BLETypeIBeacon,
				UUID:  strings.ToUpper(e.Value.UUID),
				Major: e.Value.Major,
				Minor: e.Value.Minor,
			}
		}
		reads = append(reads, scanread.Read{
			EPC:         strings.ToUpper(e.Value.MAC),
			AntennaPort: 1,
			RSSI:        e.Value.RSSI,
			BLE:         ble,
		})
	}
	return reads, nil
}
