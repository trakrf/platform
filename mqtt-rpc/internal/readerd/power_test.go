package readerd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// powerRig is a daemon with a fake host, a captured retained status, and a
// held timer: the delayed action runs only when the test fires it.
type powerRig struct {
	d       *Daemon
	events  []string // retained status docs and host calls, in order
	status  []readerrpc.StatusDoc
	pending []func()
	delays  []time.Duration
	hostErr error
	logs    bytes.Buffer
}

type powerHost struct{ r *powerRig }

func (h powerHost) RestartReaderApp(context.Context) error { return nil }
func (h powerHost) Reboot() error {
	h.r.events = append(h.r.events, "host reboot")
	return h.r.hostErr
}
func (h powerHost) Poweroff() error {
	h.r.events = append(h.r.events, "host poweroff")
	return h.r.hostErr
}

func newPowerRig(t *testing.T, withHost bool) *powerRig {
	t.Helper()
	r := &powerRig{}
	log := zerolog.New(&r.logs)
	r.d = New(Config{Broker: BrokerConfig{BaseTopic: "trakrf.id/cs463-212"}}, &fakeAdapter{}, &log)
	if withHost {
		r.d.SetHost(powerHost{r})
	}
	r.d.after = func(d time.Duration, f func()) {
		r.delays = append(r.delays, d)
		r.pending = append(r.pending, f)
	}
	r.d.publishRetained = func(_ string, b []byte) error {
		var doc readerrpc.StatusDoc
		require.NoError(t, json.Unmarshal(b, &doc))
		r.status = append(r.status, doc)
		if doc.Online {
			r.events = append(r.events, "status online")
		} else {
			r.events = append(r.events, "status offline "+doc.Reason)
		}
		return nil
	}
	return r
}

func (r *powerRig) call(t *testing.T, method, params string) readerrpc.Response {
	t.Helper()
	req := readerrpc.Request{ID: 9, Src: "trakrf-cli/abc/reply", Method: method}
	if params != "" {
		req.Params = json.RawMessage(params)
	}
	b, _ := json.Marshal(req)
	_, out := r.d.handleRPC(b)
	return decodeResponse(t, out)
}

func (r *powerRig) fire(t *testing.T) {
	t.Helper()
	require.Len(t, r.pending, 1, "exactly one delayed action should be scheduled")
	f := r.pending[0]
	r.pending = nil
	f()
}

func TestReboot_RepliesThenAnnouncesThenReboots(t *testing.T) {
	r := newPowerRig(t, true)

	resp := r.call(t, readerrpc.MethodReboot, `{"requested_by":"mike@bench"}`)
	require.Nil(t, resp.Error)
	var res readerrpc.PowerResult
	require.NoError(t, json.Unmarshal(resp.Result, &res))
	assert.Equal(t, readerrpc.PowerResult{Action: readerrpc.StatusReasonReboot, InMs: 2000}, res)
	assert.Equal(t, []time.Duration{DefaultPowerDelay}, r.delays)
	assert.Empty(t, r.events, "nothing happens before the reply has had its delay")

	r.fire(t)
	assert.Equal(t, []string{"status offline reboot", "host reboot"}, r.events)
}

func TestShutdown_RepliesThenAnnouncesThenPowersOff(t *testing.T) {
	r := newPowerRig(t, true)

	resp := r.call(t, readerrpc.MethodShutdown, "")
	require.Nil(t, resp.Error)
	var res readerrpc.PowerResult
	require.NoError(t, json.Unmarshal(resp.Result, &res))
	assert.Equal(t, readerrpc.StatusReasonShutdown, res.Action)
	assert.Empty(t, r.events)

	r.fire(t)
	assert.Equal(t, []string{"status offline shutdown", "host poweroff"}, r.events)
}

// The daemon's own Stop, which systemd's shutdown triggers, must keep the reason.
func TestPower_StopKeepsTheReason(t *testing.T) {
	r := newPowerRig(t, true)
	r.call(t, readerrpc.MethodShutdown, "")
	r.fire(t)

	var doc readerrpc.StatusDoc
	require.NoError(t, json.Unmarshal(r.d.offlinePayload(), &doc))
	assert.False(t, doc.Online)
	assert.Equal(t, readerrpc.StatusReasonShutdown, doc.Reason)

	r.d.SetHealth(readerrpc.Health{API: readerrpc.APIHealthy})
	assert.False(t, r.status[len(r.status)-1].Online, "a late health change must not announce a rebooting reader online")
}

func TestPower_SecondRequestWhilePendingIsRefused(t *testing.T) {
	r := newPowerRig(t, true)
	require.Nil(t, r.call(t, readerrpc.MethodReboot, "").Error)

	for _, m := range []string{readerrpc.MethodReboot, readerrpc.MethodShutdown} {
		resp := r.call(t, m, "")
		require.NotNil(t, resp.Error, m)
		assert.Contains(t, resp.Error.Message, "already")
	}
	r.fire(t)
	assert.Equal(t, []string{"status offline reboot", "host reboot"}, r.events, "the host is asked once")

	resp := r.call(t, readerrpc.MethodShutdown, "")
	require.NotNil(t, resp.Error, "still refused once the reader is going down")
}

func TestPower_RefusedWhileAutoRecoveryRebootPending(t *testing.T) {
	r := newPowerRig(t, true)
	r.d.GoingDown(readerrpc.StatusReasonAutoRecovery)

	resp := r.call(t, readerrpc.MethodShutdown, "")
	require.NotNil(t, resp.Error)
	assert.Contains(t, resp.Error.Message, readerrpc.StatusReasonAutoRecovery)
	assert.Empty(t, r.pending)
}

// A reboot systemd refused leaves the reader up: it must not stay announced as
// offline, and the operator must be able to try again.
func TestPower_HostFailureGoesBackOnline(t *testing.T) {
	r := newPowerRig(t, true)
	r.hostErr = errors.New("systemctl reboot: exit status 1")
	r.call(t, readerrpc.MethodReboot, "")
	r.fire(t)

	assert.Equal(t, []string{"status offline reboot", "host reboot", "status online"}, r.events)
	last := r.status[len(r.status)-1]
	assert.True(t, last.Online)
	assert.Empty(t, last.Reason)
	assert.Contains(t, r.logs.String(), "systemctl reboot: exit status 1")

	r.hostErr = nil
	require.Nil(t, r.call(t, readerrpc.MethodReboot, "").Error, "a retry is accepted")
}

func TestPower_InvalidParams(t *testing.T) {
	r := newPowerRig(t, true)
	resp := r.call(t, readerrpc.MethodReboot, `"now"`)
	require.NotNil(t, resp.Error)
	assert.Equal(t, readerrpc.CodeInvalidParams, resp.Error.Code)
	assert.Empty(t, r.pending)
	require.Nil(t, r.call(t, readerrpc.MethodReboot, "").Error, "a bad request does not leave the daemon thinking it is going down")
}

func TestPower_NoHostIsMethodNotFound(t *testing.T) {
	r := newPowerRig(t, false)
	for _, m := range []string{readerrpc.MethodReboot, readerrpc.MethodShutdown} {
		resp := r.call(t, m, "")
		require.NotNil(t, resp.Error, m)
		assert.Equal(t, readerrpc.CodeMethodNotFound, resp.Error.Code)
	}
	assert.Empty(t, r.pending)

	var caps readerrpc.Capabilities
	require.NoError(t, json.Unmarshal(r.call(t, readerrpc.MethodGetCapabilities, "").Result, &caps))
	assert.NotContains(t, caps.Supports, readerrpc.MethodReboot)
	assert.NotContains(t, caps.Supports, readerrpc.MethodShutdown)
}

func TestCapabilities_WithHostListsRebootAndShutdown(t *testing.T) {
	r := newPowerRig(t, true)
	var caps readerrpc.Capabilities
	require.NoError(t, json.Unmarshal(r.call(t, readerrpc.MethodGetCapabilities, "").Result, &caps))
	assert.Contains(t, caps.Supports, readerrpc.MethodReboot)
	assert.Contains(t, caps.Supports, readerrpc.MethodShutdown)
	assert.NotContains(t, caps.Unsupported, readerrpc.MethodReboot)
	assert.NotContains(t, caps.Unsupported, readerrpc.MethodShutdown)
}

func TestPower_LogsWhoAsked(t *testing.T) {
	r := newPowerRig(t, true)
	r.call(t, readerrpc.MethodShutdown, `{"requested_by":"mike@bench"}`)

	logs := r.logs.String()
	assert.Contains(t, logs, `"requested_by":"mike@bench"`)
	assert.Contains(t, logs, `"src":"trakrf-cli/abc/reply"`)
	assert.Contains(t, logs, `"method":"Reader.Shutdown"`)
	assert.Contains(t, logs, `"asked_at"`, "the log line carries when it was asked")
}
