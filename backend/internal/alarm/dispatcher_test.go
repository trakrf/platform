package alarm

import (
	"context"
	"testing"

	"github.com/trakrf/platform/backend/internal/models/alarmdevice"
)

type recordingHTTP struct {
	called   bool
	baseURL  string
	switchID int
	on       bool
}

func (h *recordingHTTP) Set(_ context.Context, baseURL string, switchID int, on bool) error {
	h.called, h.baseURL, h.switchID, h.on = true, baseURL, switchID, on
	return nil
}

type recordingMQTT struct {
	called   bool
	topic    string
	switchID int
	on       bool
}

func (m *recordingMQTT) Publish(_ context.Context, commandTopic string, switchID int, on bool) error {
	m.called, m.topic, m.switchID, m.on = true, commandTopic, switchID, on
	return nil
}

func strptr(s string) *string { return &s }

func TestDispatcher_HTTPDevice_UsesHTTP(t *testing.T) {
	h, m := &recordingHTTP{}, &recordingMQTT{}
	d := NewDispatcher(h, m)
	dev := alarmdevice.AlarmDevice{Transport: alarmdevice.TransportHTTP, BaseURL: "http://1.2.3.4", SwitchID: 2}

	if err := d.Set(context.Background(), dev, true); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !h.called || h.baseURL != "http://1.2.3.4" || h.switchID != 2 || h.on != true {
		t.Errorf("http not driven correctly: %+v", h)
	}
	if m.called {
		t.Error("mqtt should not be called for http device")
	}
}

func TestDispatcher_MQTTDevice_UsesMQTT(t *testing.T) {
	h, m := &recordingHTTP{}, &recordingMQTT{}
	d := NewDispatcher(h, m)
	dev := alarmdevice.AlarmDevice{Transport: alarmdevice.TransportMQTT, CommandTopic: strptr("trakrf.id/dock"), SwitchID: 1}

	if err := d.Set(context.Background(), dev, false); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !m.called || m.topic != "trakrf.id/dock" || m.switchID != 1 || m.on != false {
		t.Errorf("mqtt not driven correctly: %+v", m)
	}
	if h.called {
		t.Error("http should not be called for mqtt device")
	}
}

func TestDispatcher_MQTTDevice_NoCommandTopic_Errors(t *testing.T) {
	d := NewDispatcher(&recordingHTTP{}, &recordingMQTT{})
	dev := alarmdevice.AlarmDevice{Transport: alarmdevice.TransportMQTT} // no command_topic
	if err := d.Set(context.Background(), dev, true); err == nil {
		t.Fatal("expected error for mqtt device with no command_topic")
	}
}

func TestDispatcher_MQTTDevice_NilPublisher_Errors(t *testing.T) {
	d := NewDispatcher(&recordingHTTP{}, nil) // broker disabled
	dev := alarmdevice.AlarmDevice{Transport: alarmdevice.TransportMQTT, CommandTopic: strptr("trakrf.id/dock")}
	if err := d.Set(context.Background(), dev, true); err == nil {
		t.Fatal("expected error when mqtt transport requested but publisher is nil")
	}
}
