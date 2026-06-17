package readerrpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRequestRoundTrip(t *testing.T) {
	raw := []byte(`{"id":42,"src":"trakrf-cloud/req-7f3a","method":"Reader.SetOperProfile","params":{"antennas":[{"antenna":1,"enabled":true,"power_dbm":24.5}],"force":true}}`)

	req, err := ParseRequest(raw)
	require.NoError(t, err)
	assert.Equal(t, 42, req.ID)
	assert.Equal(t, "trakrf-cloud/req-7f3a", req.Src)
	assert.Equal(t, MethodSetOperProfile, req.Method)

	var p SetOperProfileParams
	require.NoError(t, json.Unmarshal(req.Params, &p))
	assert.True(t, p.Force)
	require.Len(t, p.Antennas, 1)
	assert.Equal(t, 1, p.Antennas[0].Antenna)
	assert.True(t, p.Antennas[0].Enabled)
	assert.Equal(t, 24.5, p.Antennas[0].PowerDBm)
}

func TestNewResult(t *testing.T) {
	req := Request{ID: 42, Src: "trakrf-cloud/req-7f3a", Method: MethodSetOperProfile}

	resp, err := NewResult(req, SetConfigResult{Applied: AppliedPendingReload})
	require.NoError(t, err)
	assert.Equal(t, 42, resp.ID)
	assert.Equal(t, "trakrf-cloud/req-7f3a", resp.Dst)
	assert.Nil(t, resp.Error)

	b, err := resp.Marshal()
	require.NoError(t, err)
	assert.Contains(t, string(b), `"applied":"pending_reload"`)
	assert.Contains(t, string(b), `"dst":"trakrf-cloud/req-7f3a"`)
	assert.Contains(t, string(b), `"id":42`)
}

func TestNewBusyError(t *testing.T) {
	req := Request{ID: 7, Src: "trakrf-cloud/req-1"}
	resp := NewBusyError(req, "192.168.50.203")
	require.NotNil(t, resp.Error)
	assert.Equal(t, CodeReaderBusy, resp.Error.Code)
	var d ReaderBusyData
	require.NoError(t, json.Unmarshal(resp.Error.Data, &d))
	assert.Equal(t, "192.168.50.203", d.HeldBy)
}

func TestReaderConfigGoldenKnobsRoundTrip(t *testing.T) {
	dwell, dedup, rssi := 500, 500, -80
	diff := true
	cfg := ReaderConfig{
		Antennas:               []AntennaConfig{{Antenna: 1, Enabled: true, PowerDBm: 30}},
		DwellMs:                &dwell,
		DedupWindowMs:          &dedup,
		RSSIGateDBm:            &rssi,
		AntennaDifferentiation: &diff,
	}
	b, err := json.Marshal(cfg)
	require.NoError(t, err)
	var got ReaderConfig
	require.NoError(t, json.Unmarshal(b, &got))
	require.NotNil(t, got.DwellMs)
	assert.Equal(t, 500, *got.DwellMs)
	require.NotNil(t, got.AntennaDifferentiation)
	assert.True(t, *got.AntennaDifferentiation)
}

func TestNewError(t *testing.T) {
	req := Request{ID: 7, Src: "trakrf-cloud/req-abc", Method: "Reader.Bogus"}

	resp := NewError(req, CodeMethodNotFound, "x")
	assert.Equal(t, 7, resp.ID)
	assert.Equal(t, "trakrf-cloud/req-abc", resp.Dst)
	require.NotNil(t, resp.Error)
	assert.Equal(t, CodeMethodNotFound, resp.Error.Code)
	assert.Equal(t, "x", resp.Error.Message)

	b, err := resp.Marshal()
	require.NoError(t, err)
	assert.Contains(t, string(b), `"code":-32601`)
	assert.Contains(t, string(b), `"message":"x"`)
	// success result must be omitted on an error response
	assert.NotContains(t, string(b), `"result"`)
}

func TestTopicHelpers(t *testing.T) {
	assert.Equal(t, "trakrf.id/cs463-212/rpc", RPCTopic("trakrf.id/cs463-212"))
	assert.Equal(t, "trakrf.id/cs463-212/status", StatusTopic("trakrf.id/cs463-212"))
}
