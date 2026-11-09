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
		Transport:       outputdevice.TransportMQTT,
		Type:            outputdevice.TypeCSLGPO,
		ReaderBaseTopic: "trakrf.id/cs463-212",
		SwitchID:        1,
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

// No ON latches, on any transport (ADR 0026, TRA-1138). An ON that names no
// on-time gets the default, and none is longer than the cap. A CS463 GPO's
// daemon enforces this too, but a Shelly has nothing between it and the cloud,
// so the dispatcher is where its on-time is bounded.
func TestDispatcher_OnTimeIsBoundedOnEveryTransport(t *testing.T) {
	gpoDev := outputdevice.OutputDevice{
		Transport:       outputdevice.TransportMQTT,
		Type:            outputdevice.TypeCSLGPO,
		ReaderBaseTopic: "trakrf.id/cs463-212",
		SwitchID:        2,
	}
	httpDev := outputdevice.OutputDevice{Transport: outputdevice.TransportHTTP, BaseURL: "http://1.2.3.4", SwitchID: 0}
	mqttDev := outputdevice.OutputDevice{Transport: outputdevice.TransportMQTT, CommandTopic: strptr("trakrf.id/dock")}

	for _, tc := range []struct {
		name      string
		requested int
		wantSec   int
	}{
		{"no on-time gets the 30 s default", 0, 30},
		{"a negative on-time gets the default", -5, 30},
		{"an on-time inside the cap is kept", 45, 45},
		{"the cap itself is kept", 60, 60},
		{"a 600 s on-time is cut to 60 s", 600, 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, m, g := &recordingHTTP{}, &recordingMQTT{}, &recordingGPO{}
			d := NewDispatcher(h, m, g)

			if err := d.Set(context.Background(), httpDev, true, tc.requested); err != nil {
				t.Fatalf("http Set: %v", err)
			}
			if h.offAfterSec != tc.wantSec {
				t.Errorf("shelly http toggle_after = %d, want %d", h.offAfterSec, tc.wantSec)
			}
			if err := d.Set(context.Background(), mqttDev, true, tc.requested); err != nil {
				t.Fatalf("mqtt Set: %v", err)
			}
			if m.offAfterSec != tc.wantSec {
				t.Errorf("shelly mqtt toggle_after = %d, want %d", m.offAfterSec, tc.wantSec)
			}
			if err := d.Set(context.Background(), gpoDev, true, tc.requested); err != nil {
				t.Fatalf("gpo Set: %v", err)
			}
			if g.pulseMs != tc.wantSec*1000 {
				t.Errorf("gpo pulse_ms = %d, want %d", g.pulseMs, tc.wantSec*1000)
			}
		})
	}
}

// An OFF carries no on-time on any transport.
func TestDispatcher_OffCarriesNoOnTime(t *testing.T) {
	h, m, g := &recordingHTTP{}, &recordingMQTT{}, &recordingGPO{}
	d := NewDispatcher(h, m, g)
	devs := []outputdevice.OutputDevice{
		{Transport: outputdevice.TransportHTTP, BaseURL: "http://1.2.3.4"},
		{Transport: outputdevice.TransportMQTT, CommandTopic: strptr("trakrf.id/dock")},
		{Transport: outputdevice.TransportMQTT, Type: outputdevice.TypeCSLGPO, ReaderBaseTopic: "trakrf.id/cs463-212", SwitchID: 1},
	}
	for _, dev := range devs {
		if err := d.Set(context.Background(), dev, false, 30); err != nil {
			t.Fatalf("Set off: %v", err)
		}
	}
	if h.offAfterSec != 0 || m.offAfterSec != 0 || g.pulseMs != 0 {
		t.Errorf("off carried an on-time: http %d, mqtt %d, gpo %d ms", h.offAfterSec, m.offAfterSec, g.pulseMs)
	}
}

func TestDispatcher_GPODevice_NilClient_Errors(t *testing.T) {
	d := NewDispatcher(&recordingHTTP{}, &recordingMQTT{}, nil) // broker disabled
	dev := outputdevice.OutputDevice{
		Transport:       outputdevice.TransportMQTT,
		Type:            outputdevice.TypeCSLGPO,
		ReaderBaseTopic: "trakrf.id/cs463-212",
		SwitchID:        1,
	}
	if err := d.Set(context.Background(), dev, true, 0); err == nil {
		t.Fatal("expected error when a gpo device is fired with no reader client")
	}
}

// TestDispatcher_GPODevice_UnresolvedReaderBaseTopic_Errors covers TRA-1028's
// closed cross-org actuation hole: a GPO device whose reader was deleted, or
// whose scan_device_id points cross-org (RLS makes the join return empty),
// resolves to an empty ReaderBaseTopic. That must be a hard error, and must
// NOT fire — command_topic is no longer consulted for GPO at all.
func TestDispatcher_GPODevice_UnresolvedReaderBaseTopic_Errors(t *testing.T) {
	g := &recordingGPO{}
	d := NewDispatcher(&recordingHTTP{}, &recordingMQTT{}, g)
	dev := outputdevice.OutputDevice{
		Transport: outputdevice.TransportMQTT,
		Type:      outputdevice.TypeCSLGPO,
		SwitchID:  1,
		// ReaderBaseTopic left empty: unresolved reader.
	}
	if err := d.Set(context.Background(), dev, true, 0); err == nil {
		t.Fatal("expected error for a gpo device with an unresolved reader base topic")
	}
	if g.called {
		t.Error("must not publish when the reader base topic is unresolved")
	}
}

func TestDispatcher_GPODevice_PortOutOfRange_Errors(t *testing.T) {
	// Defense in depth: a row predating handler validation, or edited in psql,
	// must error rather than fire the wrong port.
	for _, port := range []int{0, 5} {
		g := &recordingGPO{}
		d := NewDispatcher(&recordingHTTP{}, &recordingMQTT{}, g)
		dev := outputdevice.OutputDevice{
			Transport:       outputdevice.TransportMQTT,
			Type:            outputdevice.TypeCSLGPO,
			ReaderBaseTopic: "trakrf.id/cs463-212",
			SwitchID:        port,
		}
		if err := d.Set(context.Background(), dev, true, 0); err == nil {
			t.Errorf("port %d: expected an out-of-range error", port)
		}
		if g.called {
			t.Errorf("port %d: must not publish", port)
		}
	}
}

// TestDispatcher_GPODevice_NonMQTTTransport_Errors covers the M1 finding: a
// csl_gpo device on a non-mqtt transport must error at the top of Set,
// before the transport branch, rather than falling through to the http path
// (which would otherwise silently ignore the type and drive http.Set).
func TestDispatcher_GPODevice_NonMQTTTransport_Errors(t *testing.T) {
	h := &recordingHTTP{}
	d := NewDispatcher(h, &recordingMQTT{}, &recordingGPO{})
	dev := outputdevice.OutputDevice{
		Transport: outputdevice.TransportHTTP,
		Type:      outputdevice.TypeCSLGPO,
		BaseURL:   "http://1.2.3.4",
		SwitchID:  1,
	}
	if err := d.Set(context.Background(), dev, true, 0); err == nil {
		t.Fatal("expected error for a gpo device on a non-mqtt transport")
	}
	if h.called {
		t.Error("must not fall through to the http path for a gpo device")
	}
}
