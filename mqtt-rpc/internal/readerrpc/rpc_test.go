package readerrpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRequestRoundTrip(t *testing.T) {
	raw := []byte(`{"id":42,"src":"trakrf-cloud/req-7f3a","method":"Reader.SetConfig","params":{"tx_power_dbm":[{"antenna":1,"power":24.5}]}}`)

	req, err := ParseRequest(raw)
	require.NoError(t, err)
	assert.Equal(t, 42, req.ID)
	assert.Equal(t, "trakrf-cloud/req-7f3a", req.Src)
	assert.Equal(t, MethodSetConfig, req.Method)

	var cfg ReaderConfig
	require.NoError(t, json.Unmarshal(req.Params, &cfg))
	require.Len(t, cfg.TxPowerDBm, 1)
	assert.Equal(t, 1, cfg.TxPowerDBm[0].Antenna)
	assert.Equal(t, 24.5, cfg.TxPowerDBm[0].Power)
}

func TestNewResult(t *testing.T) {
	req := Request{ID: 42, Src: "trakrf-cloud/req-7f3a", Method: MethodSetConfig}

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
