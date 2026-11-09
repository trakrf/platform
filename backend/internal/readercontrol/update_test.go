package readercontrol

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/readerrpc"
)

func TestUpdateDaemon_SendsTheManifestAndDecodesTheResult(t *testing.T) {
	c, cp := newTestClient()
	p := readerrpc.DaemonUpdateParams{Version: "v2", URL: "https://x/y", SHA256: "ab", Signature: "c2ln"}

	type out struct {
		res readerrpc.DaemonUpdateResult
		err error
	}
	done := make(chan out, 1)
	go func() {
		res, err := c.UpdateDaemon(context.Background(), "trakrf.id/cs463-212", p)
		done <- out{res, err}
	}()

	req := waitForRequest(t, cp)
	if req.Method != readerrpc.MethodDaemonUpdate {
		t.Fatalf("method = %q", req.Method)
	}
	var sent readerrpc.DaemonUpdateParams
	if err := json.Unmarshal(req.Params, &sent); err != nil || sent != p {
		t.Fatalf("params = %+v (%v), want %+v", sent, err, p)
	}
	result, _ := json.Marshal(readerrpc.DaemonUpdateResult{Accepted: true, From: "v1", To: "v2"})
	b, _ := readerrpc.Response{ID: req.ID, Dst: req.Src, Result: result}.Marshal()
	c.deliver(b)

	select {
	case o := <-done:
		if o.err != nil {
			t.Fatal(o.err)
		}
		if o.res != (readerrpc.DaemonUpdateResult{Accepted: true, From: "v1", To: "v2"}) {
			t.Fatalf("result = %+v", o.res)
		}
	case <-time.After(time.Second):
		t.Fatal("UpdateDaemon did not return")
	}
}

func TestUpdateDaemon_RefusedMapsToTypedError(t *testing.T) {
	c, cp := newTestClient()
	errc := make(chan error, 1)
	go func() {
		_, err := c.UpdateDaemon(context.Background(), "trakrf.id/cs463-212", readerrpc.DaemonUpdateParams{Version: "v2"})
		errc <- err
	}()
	req := waitForRequest(t, cp)
	b, _ := readerrpc.Response{ID: req.ID, Dst: req.Src, Error: &readerrpc.RPCError{
		Code: readerrpc.CodeUpdateRefused, Message: "update refused: release is unsigned",
	}}.Marshal()
	c.deliver(b)

	select {
	case err := <-errc:
		var re *readerrpc.UpdateRefusedError
		if !errors.As(err, &re) {
			t.Fatalf("want *UpdateRefusedError, got %v", err)
		}
		if re.Reason != "release is unsigned" {
			t.Fatalf("reason = %q", re.Reason)
		}
	case <-time.After(time.Second):
		t.Fatal("UpdateDaemon did not return")
	}
}
