package readerd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

// A request the broker hands back because someone published it retained is
// replayed on every reconnect. A retained Reader.Reboot would reboot the reader
// every time it came back, so no retained request is served.
func TestHandleMessage_RetainedRequestIsDropped(t *testing.T) {
	r := newPowerRig(t, true)
	r.d.publish = func(string, []byte) error { return nil }
	b, _ := json.Marshal(readerrpc.Request{ID: 1, Src: "reply/1", Method: readerrpc.MethodReboot})

	r.d.handleMessage(nil, fakeMessage{payload: b, retained: true})
	assert.Empty(t, r.pending, "a retained reboot must not be scheduled")
	assert.Contains(t, r.logs.String(), "retained")

	r.d.handleMessage(nil, fakeMessage{payload: b})
	assert.Len(t, r.pending, 1, "the same request sent live is served")
}

// An automatic recovery reboot that comes due inside an operator's delay must
// not take over: a requested shutdown stays a shutdown, and the monitor does
// not spend one of its automatic reboots.
func TestPower_AutoRecoveryCannotTakeAnOperatorAction(t *testing.T) {
	r := newPowerRig(t, true)
	require.Nil(t, r.call(t, readerrpc.MethodShutdown, "").Error)

	assert.False(t, r.d.GoingDown(readerrpc.StatusReasonAutoRecovery))
	r.d.AbortGoingDown(readerrpc.StatusReasonAutoRecovery) // not its claim: no effect
	assert.Empty(t, r.events)

	require.NotNil(t, r.call(t, readerrpc.MethodReboot, "").Error, "the operator's claim still refuses a second request")
	r.fire(t)
	assert.Equal(t, []string{"status offline shutdown", "host poweroff"}, r.events)
}

func TestPower_OperatorAbortDoesNotReleaseAutoRecovery(t *testing.T) {
	r := newPowerRig(t, true)
	require.True(t, r.d.GoingDown(readerrpc.StatusReasonAutoRecovery))

	r.d.AbortGoingDown(readerrpc.StatusReasonReboot)
	assert.Equal(t, []string{"status offline auto-recovery"}, r.events, "only the owner releases the claim")
	require.NotNil(t, r.call(t, readerrpc.MethodReboot, "").Error)

	r.d.AbortGoingDown(readerrpc.StatusReasonAutoRecovery)
	assert.Equal(t, "status online", r.events[len(r.events)-1])
	require.Nil(t, r.call(t, readerrpc.MethodReboot, "").Error)
}

// A reboot in the middle of a remote update, or before the new build has
// confirmed itself, is refused; once the update has settled it is accepted.
func TestPower_RefusedDuringDaemonUpdate(t *testing.T) {
	for state, refused := range map[string]bool{
		readerrpc.UpdateStateDownloading: true,
		readerrpc.UpdateStateInstalling:  true,
		readerrpc.UpdateStatePending:     true,
		readerrpc.UpdateStateCommitted:   false,
		readerrpc.UpdateStateFailed:      false,
		readerrpc.UpdateStateRolledBack:  false,
	} {
		t.Run(state, func(t *testing.T) {
			r := newPowerRig(t, true)
			r.d.SetUpdater(&fakeUpdater{info: readerrpc.DaemonInfo{Update: &readerrpc.UpdateInfo{State: state}}})
			resp := r.call(t, readerrpc.MethodReboot, "")
			if !refused {
				require.Nil(t, resp.Error)
				return
			}
			require.NotNil(t, resp.Error)
			assert.Contains(t, resp.Error.Message, state)
			assert.Empty(t, r.pending)
			r.d.SetUpdater(&fakeUpdater{})
			require.Nil(t, r.call(t, readerrpc.MethodReboot, "").Error, "a refusal leaves no claim behind")
		})
	}
}

// An adapter that already lists the power methods must not get them twice.
func TestWithPowerMethods_NoDuplicates(t *testing.T) {
	r := newPowerRig(t, true)
	caps := r.d.withPowerMethods(readerrpc.Capabilities{Supports: []string{readerrpc.MethodReboot}})
	assert.Equal(t, []string{readerrpc.MethodReboot, readerrpc.MethodShutdown}, caps.Supports)
}

// The mirror: an update accepted inside the delay would swap the binary under
// a reboot that is already on its way.
func TestDaemonUpdate_RefusedWhileGoingDown(t *testing.T) {
	r := newPowerRig(t, true)
	u := &fakeUpdater{}
	r.d.SetUpdater(u)
	require.Nil(t, r.call(t, readerrpc.MethodReboot, "").Error)

	resp := r.call(t, readerrpc.MethodDaemonUpdate, `{"version":"v9","url":"https://x/y","sha256":"ab"}`)
	require.NotNil(t, resp.Error)
	assert.Equal(t, readerrpc.CodeUpdateRefused, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, readerrpc.StatusReasonReboot)
	assert.Empty(t, u.started)
}
