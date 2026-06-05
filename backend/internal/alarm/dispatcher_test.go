package alarm

import (
	"context"
	"testing"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

type recordingHTTP struct {
	called      bool
	baseURL     string
	switchID    int
	on          bool
	offAfterSec int
}

func (h *recordingHTTP) Set(_ context.Context, baseURL string, switchID int, on bool, offAfterSec int) error {
	h.called, h.baseURL, h.switchID, h.on, h.offAfterSec = true, baseURL, switchID, on, offAfterSec
	return nil
}

type recordingMQTT struct {
	called      bool
	topic       string
	switchID    int
	on          bool
	offAfterSec int
}

func (m *recordingMQTT) Publish(_ context.Context, commandTopic string, switchID int, on bool, offAfterSec int) error {
	m.called, m.topic, m.switchID, m.on, m.offAfterSec = true, commandTopic, switchID, on, offAfterSec
	return nil
}

func strptr(s string) *string { return &s }

func TestDispatcher_HTTPDevice_UsesHTTP(t *testing.T) {
	h, m := &recordingHTTP{}, &recordingMQTT{}
	d := NewDispatcher(h, m)
	dev := outputdevice.OutputDevice{Transport: outputdevice.TransportHTTP, BaseURL: "http://1.2.3.4", SwitchID: 2}

	if err := d.Set(context.Background(), dev, true, 45); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !h.called || h.baseURL != "http://1.2.3.4" || h.switchID != 2 || h.on != true {
		t.Errorf("http not driven correctly: %+v", h)
	}
	if h.offAfterSec != 45 {
		t.Errorf("http offAfterSec = %d, want 45 (duration forwarded)", h.offAfterSec)
	}
	if m.called {
		t.Error("mqtt should not be called for http device")
	}
}

func TestDispatcher_MQTTDevice_UsesMQTT(t *testing.T) {
	h, m := &recordingHTTP{}, &recordingMQTT{}
	d := NewDispatcher(h, m)
	dev := outputdevice.OutputDevice{Transport: outputdevice.TransportMQTT, CommandTopic: strptr("trakrf.id/dock"), SwitchID: 1}

	if err := d.Set(context.Background(), dev, true, 30); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !m.called || m.topic != "trakrf.id/dock" || m.switchID != 1 || m.on != true {
		t.Errorf("mqtt not driven correctly: %+v", m)
	}
	if m.offAfterSec != 30 {
		t.Errorf("mqtt offAfterSec = %d, want 30 (duration forwarded)", m.offAfterSec)
	}
	if h.called {
		t.Error("http should not be called for mqtt device")
	}
}

func TestDispatcher_MQTTDevice_NoCommandTopic_Errors(t *testing.T) {
	d := NewDispatcher(&recordingHTTP{}, &recordingMQTT{})
	dev := outputdevice.OutputDevice{Transport: outputdevice.TransportMQTT} // no command_topic
	if err := d.Set(context.Background(), dev, true, 0); err == nil {
		t.Fatal("expected error for mqtt device with no command_topic")
	}
}

func TestDispatcher_MQTTDevice_NilPublisher_Errors(t *testing.T) {
	d := NewDispatcher(&recordingHTTP{}, nil) // broker disabled
	dev := outputdevice.OutputDevice{Transport: outputdevice.TransportMQTT, CommandTopic: strptr("trakrf.id/dock")}
	if err := d.Set(context.Background(), dev, true, 0); err == nil {
		t.Fatal("expected error when mqtt transport requested but publisher is nil")
	}
}
