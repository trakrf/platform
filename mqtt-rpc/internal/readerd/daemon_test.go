package readerd

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

func TestRedactBrokerURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"password masked, user+host kept", "mqtt://trakrf-mqtt:s3cr3t@192.168.8.10:1883", "mqtt://trakrf-mqtt:xxxxx@192.168.8.10:1883"},
		{"mqtts password masked", "mqtts://u:p@mqtt.preview.gke.trakrf.id:8883", "mqtts://u:xxxxx@mqtt.preview.gke.trakrf.id:8883"},
		{"no userinfo unchanged", "mqtt://192.168.8.10:1883", "mqtt://192.168.8.10:1883"},
		{"user only unchanged", "mqtt://u@host:1883", "mqtt://u@host:1883"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := redactBrokerURL(c.in); got != c.want {
				t.Errorf("redactBrokerURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
	// A password must never survive redaction.
	if got := redactBrokerURL("mqtt://u:s3cr3t@host:1883"); got == "mqtt://u:s3cr3t@host:1883" {
		t.Error("redactBrokerURL leaked the password")
	}
}

// fakeAdapter is a controllable readerd.Adapter for handleRPC tests.
type fakeAdapter struct {
	caps      readerrpc.Capabilities
	getCfg    readerrpc.ReaderConfig
	setResult readerrpc.SetConfigResult
	status    readerrpc.Status
	setErr    error
	getErr    error

	lastSetCfg readerrpc.ReaderConfig
	setCalled  bool
	lastForce  bool
}

func (f *fakeAdapter) GetCapabilities(_ context.Context) (readerrpc.Capabilities, error) {
	return f.caps, nil
}
func (f *fakeAdapter) GetOperProfile(_ context.Context, force bool) (readerrpc.ReaderConfig, error) {
	f.lastForce = force
	if f.getErr != nil {
		return readerrpc.ReaderConfig{}, f.getErr
	}
	return f.getCfg, nil
}
func (f *fakeAdapter) SetOperProfile(_ context.Context, cfg readerrpc.ReaderConfig, force bool) (readerrpc.SetConfigResult, error) {
	f.setCalled = true
	f.lastSetCfg = cfg
	f.lastForce = force
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

func TestHandleRPC_SetOperProfileGood(t *testing.T) {
	a := &fakeAdapter{setResult: readerrpc.SetConfigResult{Applied: readerrpc.AppliedPendingReload}}
	d := newTestDaemon(a)

	dwell := 400
	params, _ := json.Marshal(readerrpc.SetOperProfileParams{
		ReaderConfig: readerrpc.ReaderConfig{DwellMs: &dwell},
		Force:        true,
	})
	req := readerrpc.Request{ID: 1, Src: "s", Method: readerrpc.MethodSetOperProfile, Params: params}
	payload, _ := json.Marshal(req)

	_, reply := d.handleRPC(payload)
	resp := decodeResponse(t, reply)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	if !a.setCalled {
		t.Fatal("adapter.SetOperProfile was not called")
	}
	if a.lastSetCfg.DwellMs == nil || *a.lastSetCfg.DwellMs != 400 {
		t.Errorf("dwell not threaded through: %+v", a.lastSetCfg.DwellMs)
	}
	if !a.lastForce {
		t.Error("force flag not threaded through")
	}
	var res readerrpc.SetConfigResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if res.Applied != readerrpc.AppliedPendingReload {
		t.Errorf("applied = %q, want pending_reload", res.Applied)
	}
}

func TestHandleRPC_SetOperProfileBadParams(t *testing.T) {
	a := &fakeAdapter{}
	d := newTestDaemon(a)

	// Valid outer frame, but params is a non-object that cannot unmarshal into SetOperProfileParams.
	payload := []byte(`{"id":1,"src":"s","method":"Reader.SetOperProfile","params":"not-an-object"}`)

	_, reply := d.handleRPC(payload)
	resp := decodeResponse(t, reply)
	if resp.Error == nil {
		t.Fatal("expected error for bad params")
	}
	if resp.Error.Code != readerrpc.CodeInvalidParams {
		t.Errorf("code = %d, want %d", resp.Error.Code, readerrpc.CodeInvalidParams)
	}
	if a.setCalled {
		t.Error("adapter.SetOperProfile should not be called on bad params")
	}
}

func TestHandleRPC_SetOperProfileAdapterError(t *testing.T) {
	a := &fakeAdapter{setErr: errors.New("boom")}
	d := newTestDaemon(a)

	params, _ := json.Marshal(readerrpc.SetOperProfileParams{
		ReaderConfig: readerrpc.ReaderConfig{},
	})
	req := readerrpc.Request{ID: 1, Src: "s", Method: readerrpc.MethodSetOperProfile, Params: params}
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

func TestHandleRPC_GetOperProfileGood(t *testing.T) {
	dwell := 500
	a := &fakeAdapter{getCfg: readerrpc.ReaderConfig{DwellMs: &dwell}}
	d := newTestDaemon(a)

	params, _ := json.Marshal(readerrpc.OperProfileParams{Force: false})
	req := readerrpc.Request{ID: 3, Src: "s", Method: readerrpc.MethodGetOperProfile, Params: params}
	payload, _ := json.Marshal(req)

	_, reply := d.handleRPC(payload)
	resp := decodeResponse(t, reply)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var cfg readerrpc.ReaderConfig
	if err := json.Unmarshal(resp.Result, &cfg); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if cfg.DwellMs == nil || *cfg.DwellMs != 500 {
		t.Errorf("dwell = %v, want 500", cfg.DwellMs)
	}
}

func TestDispatch_BusyMapsToReaderBusyFrame(t *testing.T) {
	// fake adapter whose GetOperProfile returns a *readerrpc.BusyError;
	// assert dispatch produces resp.Error.Code == readerrpc.CodeReaderBusy and
	// error.data.held_by carries the IP.
	a := &fakeAdapter{getErr: &readerrpc.BusyError{HeldBy: "192.168.50.203"}}
	d := newTestDaemon(a)

	req := readerrpc.Request{ID: 9, Src: "s", Method: readerrpc.MethodGetOperProfile}
	payload, _ := json.Marshal(req)

	_, reply := d.handleRPC(payload)
	resp := decodeResponse(t, reply)
	if resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Code != readerrpc.CodeReaderBusy {
		t.Errorf("code = %d, want %d (CodeReaderBusy)", resp.Error.Code, readerrpc.CodeReaderBusy)
	}
	var d2 readerrpc.ReaderBusyData
	if err := json.Unmarshal(resp.Error.Data, &d2); err != nil {
		t.Fatalf("unmarshal busy data: %v", err)
	}
	if d2.HeldBy != "192.168.50.203" {
		t.Errorf("held_by = %q, want 192.168.50.203", d2.HeldBy)
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
