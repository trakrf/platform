package readercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/readerrpc"
)

// capture is the thread-safe sink for frames published through the test seam: the
// publish closure runs on the calling goroutine (c.call's goroutine) while the
// test goroutine reads, so access must be synchronized for -race.
type capture struct {
	mu      sync.Mutex
	payload []byte
	topic   string
}

func (cp *capture) set(topic string, payload []byte) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.topic = topic
	cp.payload = payload
}

func (cp *capture) get() (string, []byte) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	return cp.topic, cp.payload
}

// newTestClient builds a Client with the publish seam wired to capture the last
// published frame, so a test can simulate the daemon reply via deliver without a
// real broker.
func newTestClient() (*Client, *capture) {
	cp := &capture{}
	c := &Client{
		log:      zerolog.Nop(),
		instance: "test-instance",
		timeout:  500 * time.Millisecond,
		pending:  make(map[int]chan readerrpc.Response),
	}
	c.publish = func(topic string, payload []byte) error {
		cp.set(topic, payload)
		return nil
	}
	return c, cp
}

func TestCall_Correlation(t *testing.T) {
	c, cp := newTestClient()

	type result struct {
		resp readerrpc.Response
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := c.call(context.Background(), "trakrf.id/cs463-212", readerrpc.MethodGetStatus, nil)
		done <- result{resp, err}
	}()

	req := waitForRequest(t, cp)
	topic, _ := cp.get()

	if topic != readerrpc.RPCTopic("trakrf.id/cs463-212") {
		t.Fatalf("published to %q, want %q", topic, readerrpc.RPCTopic("trakrf.id/cs463-212"))
	}
	if req.Method != readerrpc.MethodGetStatus {
		t.Fatalf("method = %q", req.Method)
	}
	if req.Src == "" {
		t.Fatalf("request src must be set for reply routing")
	}

	// Simulate the daemon's reply on the request's id.
	reply := readerrpc.Response{ID: req.ID, Dst: req.Src, Result: json.RawMessage(`{"online":true}`)}
	b, _ := reply.Marshal()
	c.deliver(b)

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("call returned error: %v", r.err)
		}
		if string(r.resp.Result) != `{"online":true}` {
			t.Fatalf("result = %s", r.resp.Result)
		}
	case <-time.After(time.Second):
		t.Fatal("call did not return after deliver")
	}
}

func TestCall_Timeout(t *testing.T) {
	c, _ := newTestClient()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.call(ctx, "trakrf.id/cs463-212", readerrpc.MethodGetStatus, nil)
	if err == nil {
		t.Fatal("expected timeout error when no reply delivered")
	}
	// pending must be cleaned up after timeout.
	c.mu.Lock()
	n := len(c.pending)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("pending not cleaned up after timeout: %d entries", n)
	}
}

func TestSetOperProfile_RPCErrorMapsToGoError(t *testing.T) {
	c, cp := newTestClient()

	errc := make(chan error, 1)
	go func() {
		_, err := c.SetOperProfile(context.Background(), "trakrf.id/cs463-212", readerrpc.ReaderConfig{}, false)
		errc <- err
	}()

	req := waitForRequest(t, cp)
	reply := readerrpc.Response{ID: req.ID, Dst: req.Src, Error: &readerrpc.RPCError{Code: readerrpc.CodeInvalidParams, Message: "bad power"}}
	b, _ := reply.Marshal()
	c.deliver(b)

	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("expected error from rpc Error response")
		}
		if !strings.Contains(err.Error(), "bad power") {
			t.Fatalf("error %q does not carry rpc message", err.Error())
		}
	case <-time.After(time.Second):
		t.Fatal("SetOperProfile did not return")
	}
}

func TestSetOperProfile_SuccessUnmarshalsResult(t *testing.T) {
	c, cp := newTestClient()

	type result struct {
		res readerrpc.SetConfigResult
		err error
	}
	done := make(chan result, 1)
	go func() {
		res, err := c.SetOperProfile(context.Background(), "trakrf.id/cs463-212", readerrpc.ReaderConfig{}, false)
		done <- result{res, err}
	}()

	req := waitForRequest(t, cp)
	reply, _ := readerrpc.NewResult(readerrpc.Request{ID: req.ID, Src: req.Src}, readerrpc.SetConfigResult{Applied: readerrpc.AppliedPendingReload})
	b, _ := reply.Marshal()
	c.deliver(b)

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("unexpected error: %v", r.err)
		}
		if r.res.Applied != readerrpc.AppliedPendingReload {
			t.Fatalf("applied = %q", r.res.Applied)
		}
	case <-time.After(time.Second):
		t.Fatal("SetOperProfile did not return")
	}
}

func TestGetCapabilities_Success(t *testing.T) {
	c, cp := newTestClient()

	done := make(chan readerrpc.Capabilities, 1)
	errc := make(chan error, 1)
	go func() {
		caps, err := c.GetCapabilities(context.Background(), "trakrf.id/cs463-212")
		if err != nil {
			errc <- err
			return
		}
		done <- caps
	}()

	req := waitForRequest(t, cp)
	if req.Method != readerrpc.MethodGetCapabilities {
		t.Fatalf("method = %q, want GetCapabilities", req.Method)
	}
	reply, _ := readerrpc.NewResult(readerrpc.Request{ID: req.ID, Src: req.Src}, readerrpc.Capabilities{ReaderModel: "CS463", Antennas: 4})
	b, _ := reply.Marshal()
	c.deliver(b)

	select {
	case caps := <-done:
		if caps.ReaderModel != "CS463" || caps.Antennas != 4 {
			t.Fatalf("caps = %+v", caps)
		}
	case err := <-errc:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(time.Second):
		t.Fatal("GetCapabilities did not return")
	}
}

func TestDeliver_UnknownIDIsDropped(t *testing.T) {
	c, _ := newTestClient()
	// No pending registered; deliver must not panic.
	reply := readerrpc.Response{ID: 999, Result: json.RawMessage(`{}`)}
	b, _ := reply.Marshal()
	c.deliver(b) // must be a no-op
}

func TestGetOperProfile_BusyMapsToTypedError(t *testing.T) {
	c, cp := newTestClient()
	errc := make(chan error, 1)
	go func() {
		_, err := c.GetOperProfile(context.Background(), "trakrf.id/cs463-212", false)
		errc <- err
	}()
	req := waitForRequest(t, cp)
	reply := readerrpc.NewBusyError(readerrpc.Request{ID: req.ID, Src: req.Src}, "192.168.50.203")
	b, _ := reply.Marshal()
	c.deliver(b)
	select {
	case err := <-errc:
		var be *readerrpc.BusyError
		if !errors.As(err, &be) {
			t.Fatalf("want *readerrpc.BusyError, got %v", err)
		}
		if be.HeldBy != "192.168.50.203" {
			t.Fatalf("held_by = %q", be.HeldBy)
		}
	case <-time.After(time.Second):
		t.Fatal("GetOperProfile did not return")
	}
}

func waitForRequest(t *testing.T, cp *capture) readerrpc.Request {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if _, payload := cp.get(); len(payload) > 0 {
			var req readerrpc.Request
			if err := json.Unmarshal(payload, &req); err != nil {
				t.Fatalf("published frame not valid request json: %v", err)
			}
			return req
		}
		select {
		case <-deadline:
			t.Fatal("no request published")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
