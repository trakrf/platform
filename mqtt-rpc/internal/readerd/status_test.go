package readerd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

type retained struct {
	topic string
	doc   readerrpc.StatusDoc
}

func captureRetained(t *testing.T, d *Daemon) *[]retained {
	t.Helper()
	var got []retained
	d.publishRetained = func(topic string, b []byte) error {
		var doc readerrpc.StatusDoc
		require.NoError(t, json.Unmarshal(b, &doc))
		got = append(got, retained{topic, doc})
		return nil
	}
	return &got
}

func TestSetHealth_PublishesRetainedStatusWithHealth(t *testing.T) {
	d := newTestDaemon(&fakeAdapter{})
	got := captureRetained(t, d)

	d.SetHealth(readerrpc.Health{API: readerrpc.APIDegraded, Wedged: true})

	require.Len(t, *got, 1)
	assert.Equal(t, "trakrf.id/cs463-212/status", (*got)[0].topic)
	doc := (*got)[0].doc
	assert.True(t, doc.Online)
	require.NotNil(t, doc.Health)
	assert.True(t, doc.Health.Wedged)
	assert.Equal(t, readerrpc.APIDegraded, doc.Health.API)

	// A reconnect republishes the same health, not a bare online flag.
	var online readerrpc.StatusDoc
	require.NoError(t, json.Unmarshal(d.onlinePayload(), &online))
	require.NotNil(t, online.Health)
	assert.True(t, online.Health.Wedged)
}

// systemctl reboot SIGTERMs the daemon, and Stop republishes the offline
// document. That must not overwrite the reason the reboot was announced with.
func TestOfflinePayload_KeepsReason(t *testing.T) {
	d := newTestDaemon(&fakeAdapter{})
	got := captureRetained(t, d)

	d.GoingDown(readerrpc.StatusReasonAutoRecovery)
	require.Len(t, *got, 1)
	assert.False(t, (*got)[0].doc.Online)
	assert.Equal(t, readerrpc.StatusReasonAutoRecovery, (*got)[0].doc.Reason)

	var stop readerrpc.StatusDoc
	require.NoError(t, json.Unmarshal(d.offlinePayload(), &stop))
	assert.False(t, stop.Online)
	assert.Equal(t, readerrpc.StatusReasonAutoRecovery, stop.Reason)

	// Health that changes on the way down stays offline.
	d.SetHealth(readerrpc.Health{API: readerrpc.APIDegraded})
	assert.False(t, (*got)[len(*got)-1].doc.Online)
}

func TestLastWillIsPlainOffline(t *testing.T) {
	assert.JSONEq(t, `{"online":false}`, string(lastWillPayload()))
}
