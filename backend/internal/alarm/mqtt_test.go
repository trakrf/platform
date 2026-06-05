package alarm

import (
	"context"
	"encoding/json"
	"testing"
)

// decodeFrame unmarshals the published RPC frame and returns the top-level
// fields plus the params object.
func decodeFrame(t *testing.T, payload []byte) (method string, params map[string]any) {
	t.Helper()
	var frame struct {
		ID     int            `json:"id"`
		Src    *string        `json:"src"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal(payload, &frame); err != nil {
		t.Fatalf("payload is not valid JSON: %v (%s)", err, payload)
	}
	if frame.Src != nil {
		t.Errorf("frame.src = %q, want absent (we publish-and-trust, no reply topic)", *frame.Src)
	}
	return frame.Method, frame.Params
}

func TestMQTTPublisher_PublishesSwitchSetRPC_OnRPCTopic(t *testing.T) {
	var gotTopic string
	var gotPayload []byte
	p := &MQTTPublisher{publish: func(topic string, payload []byte) error {
		gotTopic, gotPayload = topic, payload
		return nil
	}}

	if err := p.Publish(context.Background(), "trakrf.id/dock-strobe", 0, true, 0); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if gotTopic != "trakrf.id/dock-strobe/rpc" {
		t.Errorf("topic = %q, want trakrf.id/dock-strobe/rpc", gotTopic)
	}
	method, params := decodeFrame(t, gotPayload)
	if method != "Switch.Set" {
		t.Errorf("method = %q, want Switch.Set", method)
	}
	if params["id"] != float64(0) {
		t.Errorf("params.id = %v, want 0 (switch id)", params["id"])
	}
	if params["on"] != true {
		t.Errorf("params.on = %v, want true", params["on"])
	}
	if _, present := params["toggle_after"]; present {
		t.Errorf("toggle_after present with duration 0; want omitted")
	}
}

func TestMQTTPublisher_IncludesToggleAfter_WhenDurationSet(t *testing.T) {
	var gotPayload []byte
	p := &MQTTPublisher{publish: func(_ string, payload []byte) error {
		gotPayload = payload
		return nil
	}}

	if err := p.Publish(context.Background(), "trakrf.id/dock", 2, true, 30); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	_, params := decodeFrame(t, gotPayload)
	if params["id"] != float64(2) {
		t.Errorf("params.id = %v, want 2", params["id"])
	}
	if params["toggle_after"] != float64(30) {
		t.Errorf("params.toggle_after = %v, want 30", params["toggle_after"])
	}
}

func TestMQTTPublisher_OffOmitsToggleAfter(t *testing.T) {
	var gotPayload []byte
	p := &MQTTPublisher{publish: func(_ string, payload []byte) error {
		gotPayload = payload
		return nil
	}}

	// An off command never carries a flip-back timer even if a duration is passed.
	if err := p.Publish(context.Background(), "trakrf.id/dock", 0, false, 30); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	_, params := decodeFrame(t, gotPayload)
	if params["on"] != false {
		t.Errorf("params.on = %v, want false", params["on"])
	}
	if _, present := params["toggle_after"]; present {
		t.Errorf("toggle_after present on an off command; want omitted")
	}
}
