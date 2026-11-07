package readerrpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapabilitiesCarryDaemonInfo(t *testing.T) {
	caps := Capabilities{
		ContractVersion: ContractVersion,
		ReaderModel:     "csl_cs463",
		Daemon: &DaemonInfo{
			Version: "v1.5.0-240-gabc1234",
			Update: &UpdateInfo{
				State:  UpdateStateRolledBack,
				Target: "v1.5.0-241-gdef5678",
				Error:  "crash_loop",
				At:     "2026-11-08T01:02:03Z",
			},
		},
	}
	raw, err := json.Marshal(caps)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"daemon":{"version":"v1.5.0-240-gabc1234","update":{"state":"rolled_back","target":"v1.5.0-241-gdef5678","error":"crash_loop","at":"2026-11-08T01:02:03Z"}}`)

	var back Capabilities
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, caps, back)
}

func TestCapabilitiesOmitDaemonWhenAbsent(t *testing.T) {
	raw, err := json.Marshal(Capabilities{ReaderModel: "csl_cs463"})
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "daemon")
}

func TestDaemonUpdateParamsWireNames(t *testing.T) {
	var p DaemonUpdateParams
	require.NoError(t, json.Unmarshal([]byte(`{"version":"v2","url":"https://x/y","sha256":"ab","signature":"c2ln"}`), &p))
	assert.Equal(t, DaemonUpdateParams{Version: "v2", URL: "https://x/y", SHA256: "ab", Signature: "c2ln"}, p)
	assert.Equal(t, "Daemon.Update", MethodDaemonUpdate)
	assert.Equal(t, -33002, CodeUpdateRefused)
}

func TestUpdateRefusedErrorMessage(t *testing.T) {
	err := &UpdateRefusedError{Reason: "signature does not verify"}
	assert.Equal(t, "update refused: signature does not verify", err.Error())
}
