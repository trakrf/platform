package readerd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/readerrpc"
)

// fakeAdapter is a controllable readerd.Adapter for handleRPC tests.
type fakeAdapter struct {
	caps      readerrpc.Capabilities
	getCfg    readerrpc.ReaderConfig
	setResult readerrpc.SetConfigResult
	status    readerrpc.Status
	setErr    error

	lastSetCfg readerrpc.ReaderConfig
	setCalled  bool
}

func (f *fakeAdapter) GetCapabilities(_ context.Context) (readerrpc.Capabilities, error) {
	return f.caps, nil
}
func (f *fakeAdapter) GetConfig(_ context.Context) (readerrpc.ReaderConfig, error) {
	return f.getCfg, nil
}
func (f *fakeAdapter) SetConfig(_ context.Context, cfg readerrpc.ReaderConfig) (readerrpc.SetConfigResult, error) {
	f.setCalled = true
	f.lastSetCfg = cfg
	if f.setErr != nil {
		return readerrpc.SetConfigResult{}, f.setErr
	}
	return f.setResult, nil
}
func (f *fakeAdapter) GetStatus(_ context.Context) (readerrpc.Status, error) {
	return f.status, nil
}

func newTestDaemon(a *fakeAdapter) *Daemon {
	log := zerolog.Nop()
	return New(Config{Broker: BrokerConfig{BaseTopic: "trakrf.id/cs463-212"}}, a, &log)
}

func decodeResponse(t *testing.T, b []byte) readerrpc.Response {
	t.Helper()
	var resp readerrpc.Response
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

func TestHandleRPC_GetCapabilities(t *testing.T) {
	a := &fakeAdapter{caps: readerrpc.Capabilities{ReaderModel: "CS463", Antennas: 4}}
	d := newTestDaemon(a)

	req := readerrpc.Request{ID: 7, Src: "cloud/reply/abc", Method: readerrpc.MethodGetCapabilities}
	payload, _ := json.Marshal(req)

	topic, reply := d.handleRPC(payload)
	if topic != "cloud/reply/abc" {
		t.Errorf("replyTopic = %q, want cloud/reply/abc", topic)
	}
	resp := decodeResponse(t, reply)
	if resp.Dst != "cloud/reply/abc" {
		t.Errorf("Dst = %q, want cloud/reply/abc", resp.Dst)
	}
	if resp.ID != 7 {
		t.Errorf("ID = %d, want 7", resp.ID)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var caps readerrpc.Capabilities
	if err := json.Unmarshal(resp.Result, &caps); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if caps.ReaderModel != "CS463" {
		t.Errorf("reader_model = %q, want CS463", caps.ReaderModel)
	}
}

func TestHandleRPC_SetConfigGood(t *testing.T) {
	a := &fakeAdapter{setResult: readerrpc.SetConfigResult{Applied: readerrpc.AppliedPendingReload}}
	d := newTestDaemon(a)

	params := json.RawMessage(`{"region":"FCC"}`)
	req := readerrpc.Request{ID: 1, Src: "s", Method: readerrpc.MethodSetConfig, Params: params}
	payload, _ := json.Marshal(req)

	_, reply := d.handleRPC(payload)
	resp := decodeResponse(t, reply)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if !a.setCalled {
		t.Fatal("adapter.SetConfig was not called")
	}
	if a.lastSetCfg.Region == nil || *a.lastSetCfg.Region != "FCC" {
		t.Errorf("region not threaded through: %+v", a.lastSetCfg.Region)
	}
	var res readerrpc.SetConfigResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if res.Applied != readerrpc.AppliedPendingReload {
		t.Errorf("applied = %q, want pending_reload", res.Applied)
	}
}

func TestHandleRPC_SetConfigBadParams(t *testing.T) {
	a := &fakeAdapter{}
	d := newTestDaemon(a)

	// Valid outer frame, but params is a non-object that cannot unmarshal into ReaderConfig.
	payload := []byte(`{"id":1,"src":"s","method":"Reader.SetConfig","params":"not-an-object"}`)

	_, reply := d.handleRPC(payload)
	resp := decodeResponse(t, reply)
	if resp.Error == nil {
		t.Fatal("expected error for bad params")
	}
	if resp.Error.Code != readerrpc.CodeInvalidParams {
		t.Errorf("code = %d, want %d", resp.Error.Code, readerrpc.CodeInvalidParams)
	}
	if a.setCalled {
		t.Error("adapter.SetConfig should not be called on bad params")
	}
}

func TestHandleRPC_SetConfigAdapterError(t *testing.T) {
	a := &fakeAdapter{setErr: errors.New("boom")}
	d := newTestDaemon(a)

	req := readerrpc.Request{ID: 1, Src: "s", Method: readerrpc.MethodSetConfig, Params: json.RawMessage(`{"region":"FCC"}`)}
	payload, _ := json.Marshal(req)

	_, reply := d.handleRPC(payload)
	resp := decodeResponse(t, reply)
	if resp.Error == nil {
		t.Fatal("expected error from adapter")
	}
	if resp.Error.Code != readerrpc.CodeInternal {
		t.Errorf("code = %d, want %d", resp.Error.Code, readerrpc.CodeInternal)
	}
}

func TestHandleRPC_UnknownMethod(t *testing.T) {
	a := &fakeAdapter{}
	d := newTestDaemon(a)

	req := readerrpc.Request{ID: 1, Src: "s", Method: readerrpc.MethodScanStart}
	payload, _ := json.Marshal(req)

	_, reply := d.handleRPC(payload)
	resp := decodeResponse(t, reply)
	if resp.Error == nil {
		t.Fatal("expected MethodNotFound error")
	}
	if resp.Error.Code != readerrpc.CodeMethodNotFound {
		t.Errorf("code = %d, want %d", resp.Error.Code, readerrpc.CodeMethodNotFound)
	}
}

func TestHandleRPC_UnparseableDropped(t *testing.T) {
	a := &fakeAdapter{}
	d := newTestDaemon(a)

	topic, reply := d.handleRPC([]byte(`{not valid json`))
	if topic != "" || reply != nil {
		t.Errorf("expected drop (empty topic, nil reply); got topic=%q reply=%v", topic, reply)
	}
}
