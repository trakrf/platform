package ingest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/scandevice"
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
	assert.Equal(t, "cs463-214-1", r.CapturePointName)
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

// gls10_read.json is a real GL-S10 BLE gateway message captured from preview
// (device C4DEE229A176, org "Organized Chaos") on 2026-06-05: 42 BLE devices
// seen in one window. Every dev_list entry becomes a read (no filtering); the
// membership layer drops non-registered MACs from asset_scans.
func TestParseGLS10_RealCapture(t *testing.T) {
	reads, err := Parse(scandevice.DeviceTypeGLS10, loadFixture(t, "gls10_read.json"))
	require.NoError(t, err)
	require.Len(t, reads, 42)

	// The gateway is a single capture point: every read carries the derived
	// scan_point external_key {dev_ble_mac}-1 and antenna port 1.
	for _, r := range reads {
		assert.Equal(t, "C4DEE229A176-1", r.CapturePointName)
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
	assert.Equal(t, "C4DEE229A176-1", r.CapturePointName)
	assert.Equal(t, 1, r.AntennaPort)
	assert.Equal(t, 0, r.RSSI) // missing rssi -> 0
	assert.Equal(t, time.UnixMilli(1780625164824).UTC(), r.ReaderTimestamp)
}

// MACs are case-insensitive, but tags.value and scan_points.external_key are
// matched case-sensitively and registered uppercase. Normalize so a lowercase
// wire MAC still resolves to its asset and capture point.
func TestParseGLS10_MACUppercased(t *testing.T) {
	payload := []byte(`{"dev_ble_mac":"c4dee229a176","dev_list":[
		{"mac":"f95bc0ec4e56","ad":"0201","ts":1780625164824,"rssi":-57}
	]}`)
	reads, err := Parse(scandevice.DeviceTypeGLS10, payload)
	require.NoError(t, err)
	require.Len(t, reads, 1)
	assert.Equal(t, "F95BC0EC4E56", reads[0].EPC)
	assert.Equal(t, "C4DEE229A176-1", reads[0].CapturePointName)
}

func TestParseGLS10_MalformedJSON(t *testing.T) {
	_, err := Parse(scandevice.DeviceTypeGLS10, []byte("not json"))
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrUnsupportedDevice)
}
