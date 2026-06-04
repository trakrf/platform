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
	_, err := Parse(scandevice.DeviceTypeGLS10, loadFixture(t, "gls10_read.json"))
	assert.ErrorIs(t, err, ErrUnsupportedDevice)
}

func TestParseCS463_MalformedJSON(t *testing.T) {
	_, err := Parse(scandevice.DeviceTypeCS463, []byte("not json"))
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrUnsupportedDevice)
}
