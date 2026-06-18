package ingest

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/trakrf/platform/backend/internal/models/scanread"
)

// The preview demo fob's iBeacon frame from the real GL-S10 capture (gls10_read.json,
// MAC F95BC0EC4E56): flags + manufacturer-data iBeacon (UUID D5004A48…, major 2000,
// minor 1000) + tx power + a trailing service-data structure.
func TestDecodeBLEAdvert_IBeacon(t *testing.T) {
	ad := "0201061AFF4C000215D5004A48E56F4A39A48401869770118C07D003E800020A001A16ABFE500014D5004A48E56F4A39A48401869770118C07D003E8"
	b := decodeBLEAdvert(ad)
	assert.Equal(t, scanread.BLETypeIBeacon, b.Type)
	assert.Equal(t, "D5004A48E56F4A39A48401869770118C", b.UUID)
	assert.Equal(t, uint16(2000), b.Major)
	assert.Equal(t, uint16(1000), b.Minor)
}

// A second real iBeacon from the same capture (MAC 125C7DFC8D11, UUID 2686…,
// major 1, minor 0) — confirms the walk finds the manufacturer-data structure
// when it isn't the first AD structure.
func TestDecodeBLEAdvert_IBeaconSecondBeacon(t *testing.T) {
	ad := "0201041AFF4C0002152686F39CBADA4658854AA62E7E5E8B8D00010000C9"
	b := decodeBLEAdvert(ad)
	assert.Equal(t, scanread.BLETypeIBeacon, b.Type)
	assert.Equal(t, "2686F39CBADA4658854AA62E7E5E8B8D", b.UUID)
	assert.Equal(t, uint16(1), b.Major)
	assert.Equal(t, uint16(0), b.Minor)
}

// Apple manufacturer data that is NOT iBeacon (subtype 0x12, not 0x02 0x15):
// EB09B8F51BBC's "07FF4C0012020000" from the real capture must classify unknown.
func TestDecodeBLEAdvert_AppleNonIBeacon(t *testing.T) {
	b := decodeBLEAdvert("07FF4C0012020000")
	assert.Equal(t, scanread.BLETypeUnknown, b.Type)
}

// The fob's name/status ("Notify") frame from the real capture — service data,
// no manufacturer iBeacon — classifies unknown.
func TestDecodeBLEAdvert_NameFrameUnknown(t *testing.T) {
	b := decodeBLEAdvert("020106020A001216ABFE4000140B920001F95BC0EC4E56200107094E6F74696679")
	assert.Equal(t, scanread.BLETypeUnknown, b.Type)
}

// Malformed/empty/odd-length hex never panics and yields unknown.
func TestDecodeBLEAdvert_Malformed(t *testing.T) {
	for _, in := range []string{"", "ZZZZ", "1A", "1AFF4C00", "0"} {
		b := decodeBLEAdvert(in)
		assert.Equal(t, scanread.BLETypeUnknown, b.Type, "input %q", in)
	}
}
