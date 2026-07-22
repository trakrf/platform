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

type recordingGPO struct {
	called  bool
	base    string
	port    int
	on      bool
	pulseMs int
}

func (g *recordingGPO) GpoSet(_ context.Context, base string, port int, on bool, pulseMs int) error {
	g.called, g.base, g.port, g.on, g.pulseMs = true, base, port, on, pulseMs
	return nil
}

func strptr(s string) *string { return &s }

func TestDispatcher_HTTPDevice_UsesHTTP(t *testing.T) {
	h, m := &recordingHTTP{}, &recordingMQTT{}
	d := NewDispatcher(h, m, &recordingGPO{})
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
	d := NewDispatcher(h, m, &recordingGPO{})
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
	d := NewDispatcher(&recordingHTTP{}, &recordingMQTT{}, &recordingGPO{})
	dev := outputdevice.OutputDevice{Transport: outputdevice.TransportMQTT} // no command_topic
	if err := d.Set(context.Background(), dev, true, 0); err == nil {
		t.Fatal("expected error for mqtt device with no command_topic")
	}
}

func TestDispatcher_MQTTDevice_NilPublisher_Errors(t *testing.T) {
	d := NewDispatcher(&recordingHTTP{}, nil, &recordingGPO{}) // broker disabled
	dev := outputdevice.OutputDevice{Transport: outputdevice.TransportMQTT, CommandTopic: strptr("trakrf.id/dock")}
	if err := d.Set(context.Background(), dev, true, 0); err == nil {
		t.Fatal("expected error when mqtt transport requested but publisher is nil")
	}
}

func TestDispatcher_GPODevice_UsesGpoSet(t *testing.T) {
	h, m, g := &recordingHTTP{}, &recordingMQTT{}, &recordingGPO{}
	d := NewDispatcher(h, m, g)
	dev := outputdevice.OutputDevice{
		Transport:    outputdevice.TransportMQTT,
		Type:         outputdevice.TypeCS463GPO,
		CommandTopic: strptr("trakrf.id/cs463-212"),
		SwitchID:     1,
	}

	if err := d.Set(context.Background(), dev, true, 30); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !g.called || g.base != "trakrf.id/cs463-212" || g.port != 1 || !g.on {
		t.Errorf("gpo not driven correctly: %+v", g)
	}
	if g.pulseMs != 30000 {
		t.Errorf("pulseMs = %d, want 30000 (offAfterSec seconds -> ms)", g.pulseMs)
	}
	if m.called {
		t.Error("the shelly publisher must not be called for a gpo device")
	}
}

func TestDispatcher_GPODevice_ZeroOffAfter_NoPulse(t *testing.T) {
	// Presence mode passes offAfterSec=0 because the engine owns the OFF edge;
	// that must arrive as pulse_ms=0 (latch on), not as a pulse.
	g := &recordingGPO{}
	d := NewDispatcher(&recordingHTTP{}, &recordingMQTT{}, g)
	dev := outputdevice.OutputDevice{
		Transport:    outputdevice.TransportMQTT,
		Type:         outputdevice.TypeCS463GPO,
		CommandTopic: strptr("trakrf.id/cs463-212"),
		SwitchID:     2,
	}
	if err := d.Set(context.Background(), dev, true, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if g.pulseMs != 0 {
		t.Errorf("pulseMs = %d, want 0", g.pulseMs)
	}
}

func TestDispatcher_GPODevice_NilClient_Errors(t *testing.T) {
	d := NewDispatcher(&recordingHTTP{}, &recordingMQTT{}, nil) // broker disabled
	dev := outputdevice.OutputDevice{
		Transport:    outputdevice.TransportMQTT,
		Type:         outputdevice.TypeCS463GPO,
		CommandTopic: strptr("trakrf.id/cs463-212"),
		SwitchID:     1,
	}
	if err := d.Set(context.Background(), dev, true, 0); err == nil {
		t.Fatal("expected error when a gpo device is fired with no reader client")
	}
}

func TestDispatcher_GPODevice_NoCommandTopic_Errors(t *testing.T) {
	d := NewDispatcher(&recordingHTTP{}, &recordingMQTT{}, &recordingGPO{})
	dev := outputdevice.OutputDevice{
		Transport: outputdevice.TransportMQTT,
		Type:      outputdevice.TypeCS463GPO,
		SwitchID:  1,
	}
	if err := d.Set(context.Background(), dev, true, 0); err == nil {
		t.Fatal("expected error for a gpo device with no command_topic")
	}
}

func TestDispatcher_GPODevice_PortOutOfRange_Errors(t *testing.T) {
	// Defense in depth: a row predating handler validation, or edited in psql,
	// must error rather than fire the wrong port.
	for _, port := range []int{0, 5} {
		g := &recordingGPO{}
		d := NewDispatcher(&recordingHTTP{}, &recordingMQTT{}, g)
		dev := outputdevice.OutputDevice{
			Transport:    outputdevice.TransportMQTT,
			Type:         outputdevice.TypeCS463GPO,
			CommandTopic: strptr("trakrf.id/cs463-212"),
			SwitchID:     port,
		}
		if err := d.Set(context.Background(), dev, true, 0); err == nil {
			t.Errorf("port %d: expected an out-of-range error", port)
		}
		if g.called {
			t.Errorf("port %d: must not publish", port)
		}
	}
}
