package readerd

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
	"github.com/trakrf/platform/mqtt-rpc/internal/update"
)

type fakeUpdater struct {
	mu        sync.Mutex
	probation bool
	commits   int
	commitErr error
	started   []readerrpc.DaemonUpdateParams
	startErr  error
	info      readerrpc.DaemonInfo
}

func (f *fakeUpdater) Start(p readerrpc.DaemonUpdateParams) (readerrpc.DaemonUpdateResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, p)
	if f.startErr != nil {
		return readerrpc.DaemonUpdateResult{}, f.startErr
	}
	return readerrpc.DaemonUpdateResult{Accepted: true, From: "v1", To: p.Version}, nil
}
func (f *fakeUpdater) Info() readerrpc.DaemonInfo { return f.info }
func (f *fakeUpdater) OnProbation() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.probation
}
func (f *fakeUpdater) Commit() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commits++
	f.probation = false
	return f.commitErr
}
func (f *fakeUpdater) commitCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commits
}

func updateRequest(t *testing.T, params any) []byte {
	t.Helper()
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	b, err := json.Marshal(readerrpc.Request{ID: 9, Src: "cloud/reply/9", Method: readerrpc.MethodDaemonUpdate, Params: raw})
	require.NoError(t, err)
	return b
}

func TestDispatch_DaemonUpdateRoutesToUpdater(t *testing.T) {
	a := &fakeAdapter{}
	d := newTestDaemon(a)
	u := &fakeUpdater{}
	d.SetUpdater(u)

	p := readerrpc.DaemonUpdateParams{Version: "v2", URL: "https://x/y", SHA256: "ab", Signature: "c2ln"}
	_, reply := d.handleRPC(updateRequest(t, p))
	resp := decodeResponse(t, reply)
	require.Nil(t, resp.Error)
	var res readerrpc.DaemonUpdateResult
	require.NoError(t, json.Unmarshal(resp.Result, &res))
	assert.Equal(t, readerrpc.DaemonUpdateResult{Accepted: true, From: "v1", To: "v2"}, res)
	assert.Equal(t, []readerrpc.DaemonUpdateParams{p}, u.started)
	assert.False(t, a.setCalled, "an update never drives the reader adapter")
}

func TestDispatch_DaemonUpdateRefusedMapsToCode(t *testing.T) {
	d := newTestDaemon(&fakeAdapter{})
	d.SetUpdater(&fakeUpdater{startErr: &readerrpc.UpdateRefusedError{Reason: "release is unsigned"}})
	_, reply := d.handleRPC(updateRequest(t, readerrpc.DaemonUpdateParams{Version: "v2"}))
	resp := decodeResponse(t, reply)
	require.NotNil(t, resp.Error)
	assert.Equal(t, readerrpc.CodeUpdateRefused, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "release is unsigned")
}

func TestDispatch_DaemonUpdateBadParams(t *testing.T) {
	d := newTestDaemon(&fakeAdapter{})
	d.SetUpdater(&fakeUpdater{})
	_, reply := d.handleRPC(updateRequest(t, "not an object"))
	assert.Equal(t, readerrpc.CodeInvalidParams, decodeResponse(t, reply).Error.Code)
}

func TestDispatch_DaemonUpdateWithoutUpdaterIsMethodNotFound(t *testing.T) {
	d := newTestDaemon(&fakeAdapter{})
	_, reply := d.handleRPC(updateRequest(t, readerrpc.DaemonUpdateParams{Version: "v2"}))
	assert.Equal(t, readerrpc.CodeMethodNotFound, decodeResponse(t, reply).Error.Code)
}

func TestGetCapabilitiesReportsDaemonInfoAndUpdateSupport(t *testing.T) {
	a := &fakeAdapter{caps: readerrpc.Capabilities{ReaderModel: "csl_cs463", Supports: []string{readerrpc.MethodGetCapabilities}}}
	d := newTestDaemon(a)
	d.SetUpdater(&fakeUpdater{info: readerrpc.DaemonInfo{Version: "v2", Update: &readerrpc.UpdateInfo{State: readerrpc.UpdateStatePending, Target: "v2"}}})

	req, _ := json.Marshal(readerrpc.Request{ID: 1, Src: "s", Method: readerrpc.MethodGetCapabilities})
	_, reply := d.handleRPC(req)
	var caps readerrpc.Capabilities
	require.NoError(t, json.Unmarshal(decodeResponse(t, reply).Result, &caps))
	require.NotNil(t, caps.Daemon)
	assert.Equal(t, "v2", caps.Daemon.Version)
	assert.Equal(t, readerrpc.UpdateStatePending, caps.Daemon.Update.State)
	assert.Contains(t, caps.Supports, readerrpc.MethodDaemonUpdate)
	assert.Equal(t, []string{readerrpc.MethodGetCapabilities}, a.caps.Supports, "the adapter's slice is not mutated")
}

func TestGetCapabilitiesWithoutUpdaterStillReportsVersion(t *testing.T) {
	d := newTestDaemon(&fakeAdapter{})
	d.cfg.Version = "v7"
	req, _ := json.Marshal(readerrpc.Request{ID: 1, Src: "s", Method: readerrpc.MethodGetCapabilities})
	_, reply := d.handleRPC(req)
	var caps readerrpc.Capabilities
	require.NoError(t, json.Unmarshal(decodeResponse(t, reply).Result, &caps))
	require.NotNil(t, caps.Daemon)
	assert.Equal(t, "v7", caps.Daemon.Version)
	assert.NotContains(t, caps.Supports, readerrpc.MethodDaemonUpdate)
}

// The commit criterion: on probation, the daemon sends itself an RPC through the
// broker; once it has answered one (its reply accepted by the broker), it commits.
// No vendor /API is involved — a wedged reader app must not block a commit.
func TestProbation_SelfTestRoundTripCommitsOnce(t *testing.T) {
	d := newTestDaemon(&fakeAdapter{})
	d.commitAfter = 0 // the uptime floor has its own test
	u := &fakeUpdater{probation: true}
	d.SetUpdater(u)

	type pub struct {
		topic   string
		payload []byte
	}
	published := make(chan pub, 8)
	d.publish = func(topic string, payload []byte) error {
		published <- pub{topic, payload}
		return nil
	}

	require.NoError(t, d.selfTest())
	req := <-published
	assert.Equal(t, "trakrf.id/cs463-212/rpc", req.topic, "the self-test goes out through the broker to our own rpc topic")
	parsed, err := readerrpc.ParseRequest(req.payload)
	require.NoError(t, err)
	assert.Equal(t, readerrpc.MethodGetCapabilities, parsed.Method, "a method that never touches the vendor /API")
	assert.Equal(t, "trakrf.id/cs463-212/selftest", parsed.Src)
	assert.Equal(t, 0, u.commitCount(), "publishing the request is not proof of a round trip")

	// The broker delivers it back to us; we answer it.
	d.handleMessage(nil, fakeMessage{payload: req.payload})
	reply := <-published
	assert.Equal(t, "trakrf.id/cs463-212/selftest", reply.topic)
	require.Eventually(t, func() bool { return u.commitCount() == 1 }, 2*time.Second, 5*time.Millisecond)

	d.handleMessage(nil, fakeMessage{payload: req.payload})
	<-published
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 1, u.commitCount(), "commit happens once")
}

// A build must stay up and answering for a while before it commits: one that
// connects, answers, then crashes or wedges a minute later would otherwise be
// committed — and a committed build that hangs is only recoverable over SSH.
func TestProbation_NoCommitBeforeTheMinimumUptime(t *testing.T) {
	d := newTestDaemon(&fakeAdapter{})
	u := &fakeUpdater{probation: true}
	d.SetUpdater(u)
	start := d.started
	clock := start.Add(time.Minute)
	d.now = func() time.Time { return clock }
	d.publish = func(string, []byte) error { return nil }

	d.answered()
	assert.Equal(t, 0, u.commitCount(), "1 min of uptime is not enough")

	clock = start.Add(DefaultCommitAfter)
	d.answered()
	assert.Equal(t, 1, u.commitCount())
	assert.GreaterOrEqual(t, DefaultCommitAfter, 2*time.Minute)
	assert.Less(t, DefaultCommitAfter, 15*time.Minute, "well inside the guard's 15-20 min no-commit window")
}

// Two replies race through answered(): both saw probation, and the loser's
// Commit reports the winner's commit. That is not a failure and must not be
// logged as one.
func TestAnswered_LosingACommitRaceToItselfIsNotAnError(t *testing.T) {
	var buf bytes.Buffer
	log := zerolog.New(&buf)
	d := New(Config{Broker: BrokerConfig{BaseTopic: "trakrf.id/cs463-212"}}, &fakeAdapter{}, &log)
	u := &fakeUpdater{probation: true, commitErr: update.ErrAlreadyCommitted}
	d.SetUpdater(u)
	d.now = func() time.Time { return d.started.Add(DefaultCommitAfter) }

	d.answered()
	assert.Equal(t, 1, u.commitCount())
	assert.NotContains(t, buf.String(), "commit failed")
	assert.NotContains(t, buf.String(), `"level":"error"`)
}

// The guard ended probation without a rollback (nothing to roll back to). A
// commit then finds no pending; saying the guard will roll back would be false.
func TestAnswered_ProbationSettledByTheGuardIsNotARollbackWarning(t *testing.T) {
	var buf bytes.Buffer
	log := zerolog.New(&buf)
	d := New(Config{Broker: BrokerConfig{BaseTopic: "trakrf.id/cs463-212"}}, &fakeAdapter{}, &log)
	u := &fakeUpdater{probation: true, commitErr: update.ErrNotPending}
	d.SetUpdater(u)
	d.now = func() time.Time { return d.started.Add(DefaultCommitAfter) }

	d.answered()
	assert.NotContains(t, buf.String(), "will roll back")
	assert.NotContains(t, buf.String(), `"level":"error"`)
	assert.Contains(t, buf.String(), "the guard settled")
}

func TestNoCommitWhenReplyPublishFails(t *testing.T) {
	d := newTestDaemon(&fakeAdapter{})
	u := &fakeUpdater{probation: true}
	d.SetUpdater(u)
	done := make(chan struct{}, 1)
	d.publish = func(string, []byte) error { done <- struct{}{}; return assert.AnError }

	req, _ := json.Marshal(readerrpc.Request{ID: 1, Src: "s", Method: readerrpc.MethodGetCapabilities})
	d.handleMessage(nil, fakeMessage{payload: req})
	<-done
	time.Sleep(20 * time.Millisecond)
	assert.Equal(t, 0, u.commitCount())
}

func TestSelfTestIsANoOpOffProbation(t *testing.T) {
	d := newTestDaemon(&fakeAdapter{})
	d.SetUpdater(&fakeUpdater{})
	called := false
	d.publish = func(string, []byte) error { called = true; return nil }
	require.NoError(t, d.selfTest())
	assert.False(t, called)
}

func TestOnlinePayloadCarriesVersion(t *testing.T) {
	d := newTestDaemon(&fakeAdapter{})
	d.cfg.Version = "v1.5.0-240-gabc1234"
	var st map[string]any
	require.NoError(t, json.Unmarshal(d.onlinePayload(), &st))
	assert.Equal(t, true, st["online"])
	assert.Equal(t, "v1.5.0-240-gabc1234", st["version"])
}
