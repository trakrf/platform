package ingest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanread"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testutil", "fixtures", name))
	require.NoError(t, err)
	return b
}

func TestParseCS463_SingleTag(t *testing.T) {
	reads, err := Parse(scandevice.DeviceTypeCS463, loadFixture(t, "cs463_read.json"))
	require.NoError(t, err)
	require.Len(t, reads, 1)
	r := reads[0]
	assert.Equal(t, "E2801190A503006543E21224", r.EPC)
	assert.Equal(t, 1, r.AntennaPort)
	assert.Equal(t, -56, r.RSSI)
	// timeStampOfRead is microseconds since epoch.
	assert.Equal(t, time.UnixMicro(1780587173668000).UTC(), r.ReaderTimestamp.UTC())
}

func TestParseCS463_MultiTag(t *testing.T) {
	reads, err := Parse(scandevice.DeviceTypeCS463, loadFixture(t, "cs463_read_multi.json"))
	require.NoError(t, err)
	require.Len(t, reads, 2)
	assert.Equal(t, "712AC12F1007000000224403", reads[0].EPC)
	assert.Equal(t, -70, reads[0].RSSI)
	assert.Equal(t, "E2801190A503006543E0E3A4", reads[1].EPC)
}

func TestParse_UnsupportedDevice(t *testing.T) {
	// CS108 is a registered device type with no parser yet.
	_, err := Parse(scandevice.DeviceTypeCS108, []byte(`{}`))
	assert.ErrorIs(t, err, ErrUnsupportedDevice)
}

func TestParseCS463_MalformedJSON(t *testing.T) {
	_, err := Parse(scandevice.DeviceTypeCS463, []byte("not json"))
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrUnsupportedDevice)
}

// RSSI is informational; a malformed value must not discard an otherwise-valid
// read or fail the whole batch. Float/blank/garbage all yield a usable read.
func TestParseCS463_LenientRSSI(t *testing.T) {
	payload := []byte(`{"tags":[
		{"epc":"AA","capturePointName":"cp-1","antennaPort":1,"rssi":"-56.5"},
		{"epc":"BB","capturePointName":"cp-1","antennaPort":1,"rssi":""},
		{"epc":"CC","capturePointName":"cp-1","antennaPort":1,"rssi":"garbage"}
	]}`)
	reads, err := Parse(scandevice.DeviceTypeCS463, payload)
	require.NoError(t, err)
	require.Len(t, reads, 3)
	assert.Equal(t, -57, reads[0].RSSI) // -56.5 rounds away from zero to -57
	assert.Equal(t, 0, reads[1].RSSI)   // blank -> 0
	assert.Equal(t, 0, reads[2].RSSI)   // unparseable -> 0, read still kept
	assert.Equal(t, "CC", reads[2].EPC)
}

// TRA-994: the golden TrakRF-data-format emits rssi as a numeric (RSSI_Number)
// rather than a quoted string, matching the type the backend stores. The parser
// must accept BOTH so the format can roll out per-reader without a flag day, and
// stay lenient on a bad numeric (NaN-ish handled as 0).
func TestParseCS463_NumericRSSI(t *testing.T) {
	payload := []byte(`{"tags":[
		{"epc":"AA","antennaPort":1,"rssi":-55},
		{"epc":"BB","antennaPort":2,"rssi":-70.4},
		{"epc":"CC","antennaPort":1,"rssi":0}
	]}`)
	reads, err := Parse(scandevice.DeviceTypeCS463, payload)
	require.NoError(t, err)
	require.Len(t, reads, 3)
	assert.Equal(t, -55, reads[0].RSSI)
	assert.Equal(t, -70, reads[1].RSSI) // -70.4 rounds to -70
	assert.Equal(t, 0, reads[2].RSSI)
}

// gls10_read.json is a real GL-S10 BLE gateway message captured from preview
// (device C4DEE229A176, org "Organized Chaos") on 2026-06-05: 42 BLE devices
// seen in one window. Every dev_list entry becomes a read (no filtering); the
// membership layer drops non-registered MACs from asset_scans.
func TestParseGLS10_RealCapture(t *testing.T) {
	reads, err := Parse(scandevice.DeviceTypeGLS10, loadFixture(t, "gls10_read.json"))
	require.NoError(t, err)
	require.Len(t, reads, 42)

	// The gateway is a single capture point: every read resolves to antenna 1
	// (TRA-956).
	for _, r := range reads {
		assert.Equal(t, 1, r.AntennaPort)
	}

	// EPC is the BLE device MAC. The one registered asset tag (MAC F95BC0EC4E56,
	// a MokoSmart beacon) emits two advertisements per window -> two reads.
	matches, foundNotify := 0, false
	for _, r := range reads {
		if r.EPC != "F95BC0EC4E56" {
			continue
		}
		matches++
		if r.ReaderTimestamp.Equal(time.UnixMilli(1780625164824).UTC()) {
			foundNotify = true
			assert.Equal(t, -57, r.RSSI)
		}
	}
	assert.Equal(t, 2, matches, "asset tag broadcasts two ads -> two reads")
	assert.True(t, foundNotify, "named advertisement parsed with ms timestamp")

	// TRA-926: each read now carries a decoded BLE advert. The fob F95BC0EC4E56
	// emits two frames this window — a "Notify" name frame (unknown) and an
	// iBeacon frame (UUID D5004A48…, major 2000, minor 1000). 125C7DFC8D11 is a
	// second iBeacon (UUID 2686…). Apple non-iBeacon ads (e.g. EB09B8F51BBC) are
	// unknown.
	var fobIBeacon, fobUnknown bool
	for _, r := range reads {
		require.NotNil(t, r.BLE, "every GL-S10 read carries a BLE advert")
		if r.EPC == "F95BC0EC4E56" {
			switch r.BLE.Type {
			case scanread.BLETypeIBeacon:
				fobIBeacon = true
				assert.Equal(t, "D5004A48E56F4A39A48401869770118C", r.BLE.UUID)
				assert.Equal(t, uint16(2000), r.BLE.Major)
				assert.Equal(t, uint16(1000), r.BLE.Minor)
			case scanread.BLETypeUnknown:
				fobUnknown = true
			}
		}
		if r.EPC == "EB09B8F51BBC" {
			assert.Equal(t, scanread.BLETypeUnknown, r.BLE.Type, "Apple non-iBeacon ad")
		}
	}
	assert.True(t, fobIBeacon, "fob emits an iBeacon frame")
	assert.True(t, fobUnknown, "fob also emits a non-iBeacon name frame")
}

func TestParseGLS10_Empty(t *testing.T) {
	reads, err := Parse(scandevice.DeviceTypeGLS10, []byte(`{"dev_ble_mac":"C4DEE229A176","dev_list":[]}`))
	require.NoError(t, err)
	assert.Empty(t, reads)
}

// ts is milliseconds since epoch (CS463 uses microseconds); a missing rssi or
// name must still yield a usable read.
func TestParseGLS10_MillisTimestampAndDefaults(t *testing.T) {
	payload := []byte(`{"dev_ble_mac":"C4DEE229A176","dev_list":[
		{"mac":"AABBCCDDEEFF","ad":"0201","ts":1780625164824}
	]}`)
	reads, err := Parse(scandevice.DeviceTypeGLS10, payload)
	require.NoError(t, err)
	require.Len(t, reads, 1)
	r := reads[0]
	assert.Equal(t, "AABBCCDDEEFF", r.EPC)
	assert.Equal(t, 1, r.AntennaPort)
	assert.Equal(t, 0, r.RSSI) // missing rssi -> 0
	assert.Equal(t, time.UnixMilli(1780625164824).UTC(), r.ReaderTimestamp)
}

// MACs are case-insensitive on the wire but tags.value is registered uppercase
// and matched case-insensitively (TRA-944). Normalize the read EPC so a
// lowercase wire MAC still resolves to its asset.
func TestParseGLS10_MACUppercased(t *testing.T) {
	payload := []byte(`{"dev_ble_mac":"c4dee229a176","dev_list":[
		{"mac":"f95bc0ec4e56","ad":"0201","ts":1780625164824,"rssi":-57}
	]}`)
	reads, err := Parse(scandevice.DeviceTypeGLS10, payload)
	require.NoError(t, err)
	require.Len(t, reads, 1)
	assert.Equal(t, "F95BC0EC4E56", reads[0].EPC)
}

func TestParseGLS10_MalformedJSON(t *testing.T) {
	_, err := Parse(scandevice.DeviceTypeGLS10, []byte("not json"))
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrUnsupportedDevice)
}

// mk107_read.json is a real MOKO MK107 Pro scan report captured from preview
// (gateway 409151A65C96 / mk107-5c96) on 2026-06-08: a msg_id 3004 batch with
// six heard advertisements (five raw "Unknown" + one pre-decoded iBeacon).
// Every data[] entry becomes one read; the membership layer drops unregistered
// MACs from asset_scans.
func TestParseMK107_RealCapture(t *testing.T) {
	reads, err := Parse(scandevice.DeviceTypeMK107, loadFixture(t, "mk107_read.json"))
	require.NoError(t, err)
	require.Len(t, reads, 6)

	// The gateway is a single capture point: every read resolves to antenna 1
	// (TRA-956).
	for _, r := range reads {
		assert.Equal(t, 1, r.AntennaPort)
		// The MK107 timestamp is non-standard and intentionally ignored; server
		// receivedAt is authoritative, so ReaderTimestamp is left zero.
		assert.True(t, r.ReaderTimestamp.IsZero(), "MK107 timestamp is ignored")
	}

	// EPC is the heard device MAC. The registered demo fob (MAC F95BC0EC4E56)
	// is the first entry, an "Unknown" raw advertisement at -53 dBm.
	r := reads[0]
	assert.Equal(t, "F95BC0EC4E56", r.EPC)
	assert.Equal(t, -53, r.RSSI)

	// The pre-decoded iBeacon entry is read identically: EPC = its MAC.
	var foundIBeacon bool
	for _, r := range reads {
		if r.EPC == "12F534BA0D99" {
			foundIBeacon = true
			assert.Equal(t, -58, r.RSSI)
		}
	}
	assert.True(t, foundIBeacon, "iBeacon entry parsed by MAC like any other read")
}

// Status/heartbeat frames (msg_id 3003) carry an object `data`, not the scan
// array; they must be ignored without error and yield no reads.
func TestParseMK107_IgnoresHeartbeat(t *testing.T) {
	payload := []byte(`{"msg_id":3003,"device_info":{"device_id":"409151a65c96","mac":"409151A65C96"},"data":{"net_state":"online"}}`)
	reads, err := Parse(scandevice.DeviceTypeMK107, payload)
	require.NoError(t, err)
	assert.Empty(t, reads)
}

func TestParseMK107_Empty(t *testing.T) {
	payload := []byte(`{"msg_id":3004,"device_info":{"mac":"409151A65C96"},"data":[]}`)
	reads, err := Parse(scandevice.DeviceTypeMK107, payload)
	require.NoError(t, err)
	assert.Empty(t, reads)
}

// MACs are case-insensitive on the wire but tags.value is registered uppercase
// and matched case-insensitively (TRA-944). Normalize the heard MAC so a
// lowercase wire payload still resolves to its asset.
func TestParseMK107_MACUppercased(t *testing.T) {
	payload := []byte(`{"msg_id":3004,"device_info":{"mac":"409151a65c96"},"data":[
		{"type":0,"value":{"mac":"f95bc0ec4e56","rssi":-53}}
	]}`)
	reads, err := Parse(scandevice.DeviceTypeMK107, payload)
	require.NoError(t, err)
	require.Len(t, reads, 1)
	assert.Equal(t, "F95BC0EC4E56", reads[0].EPC)
}

func TestParseMK107_MalformedJSON(t *testing.T) {
	_, err := Parse(scandevice.DeviceTypeMK107, []byte("not json"))
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrUnsupportedDevice)
}
