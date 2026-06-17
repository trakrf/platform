package ingest

import (
	"encoding/hex"
	"strings"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// iBeacon manufacturer-data prefix: Apple company id 0x004C (little-endian on the
// wire: 0x4C 0x00), then the iBeacon type 0x02 and length 0x15 (21 bytes of
// UUID+major+minor+txpower).
const iBeaconDataLen = 25 // company(2)+type(1)+len(1)+uuid(16)+major(2)+minor(2)+tx(1); requires the trailing TX-power byte per spec, so payloads missing it classify as unknown

// decodeBLEAdvert classifies a GL-S10 raw advertisement (hex-encoded BLE AD
// structures) as an iBeacon or unknown. It walks length-prefixed AD structures
// ([len][type][len-1 data bytes]) and returns an iBeacon BLEAdvert for the first
// manufacturer-specific structure (type 0xFF) carrying the Apple+iBeacon prefix;
// otherwise unknown. Malformed or truncated input yields unknown without panic.
// Never returns nil.
func decodeBLEAdvert(adHex string) *scanread.BLEAdvert {
	raw, err := hex.DecodeString(adHex)
	if err != nil {
		return &scanread.BLEAdvert{Type: scanread.BLETypeUnknown}
	}
	for i := 0; i+1 < len(raw); {
		adLen := int(raw[i])
		if adLen == 0 || i+1+adLen > len(raw) {
			break
		}
		adType := raw[i+1]
		data := raw[i+2 : i+1+adLen]
		if adType == 0xFF && len(data) >= iBeaconDataLen &&
			data[0] == 0x4C && data[1] == 0x00 && // Apple company id (LE 0x004C)
			data[2] == 0x02 && data[3] == 0x15 { // iBeacon type + length
			return &scanread.BLEAdvert{
				Type:  scanread.BLETypeIBeacon,
				UUID:  strings.ToUpper(hex.EncodeToString(data[4:20])),
				Major: uint16(data[20])<<8 | uint16(data[21]),
				Minor: uint16(data[22])<<8 | uint16(data[23]),
			}
		}
		i += 1 + adLen
	}
	return &scanread.BLEAdvert{Type: scanread.BLETypeUnknown}
}
