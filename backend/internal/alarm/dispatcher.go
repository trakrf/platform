package alarm

import (
	"context"
	"fmt"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

// gpoPortMin/gpoPortMax bound the CS463's general purpose output ports.
const (
	gpoPortMin = 1
	gpoPortMax = 4
)

// httpSetter drives a device over local HTTP; *shelly.Client satisfies it.
type httpSetter interface {
	Set(ctx context.Context, baseURL string, switchID int, on bool, offAfterSec int) error
}

// mqttPublisher drives a Shelly by publishing a Switch.Set frame to the shared
// broker; *MQTTPublisher satisfies it. nil means MQTT is unavailable.
type mqttPublisher interface {
	Publish(ctx context.Context, commandTopic string, switchID int, on bool, offAfterSec int) error
}

// gpoSetter drives a reader's general purpose output by publishing a Gpo.Set
// frame; *readercontrol.Client satisfies it. nil means the broker is disabled.
type gpoSetter interface {
	GpoSet(ctx context.Context, base string, port int, on bool, pulseMs int) error
}

// Dispatcher routes a fire to the right transport and frame per device. Two
// axes (TRA-1028): transport says how the device is reached (http = local HTTP,
// mqtt = the shared broker), type says what frame it speaks (shelly_gen4 =
// Switch.Set, csl_cs463_gpo = Gpo.Set). It is the single
// Set(device, on, offAfterSec) seam used by both the geofence Firer and the
// output-device test-fire/reset handlers.
type Dispatcher struct {
	http httpSetter
	mqtt mqttPublisher // may be nil when the broker is disabled
	gpo  gpoSetter     // may be nil when the broker is disabled
}

// NewDispatcher builds a Dispatcher. mqtt and gpo may be nil (devices needing
// them then fail with a clear error; http devices are unaffected).
func NewDispatcher(http httpSetter, mqtt mqttPublisher, gpo gpoSetter) Dispatcher {
	return Dispatcher{http: http, mqtt: mqtt, gpo: gpo}
}

// Set drives the device on/off using its configured transport and type.
// offAfterSec, when on and > 0, is the DEVICE-side flip-back timer in seconds
// (Shelly toggle_after; the reader's own one-shot for a GPO); 0 means it stays
// on until an explicit off. Because the timer runs on the device, the OFF edge
// survives a backend restart and needs no second message.
func (d Dispatcher) Set(ctx context.Context, dev outputdevice.OutputDevice, on bool, offAfterSec int) error {
	// M1: a cs463 gpo is reader-addressed over mqtt only. Reject a non-mqtt
	// transport here, before the transport branch, rather than falling through
	// to the http/Shelly path below.
	if dev.Type == outputdevice.TypeCS463GPO && dev.Transport != outputdevice.TransportMQTT {
		return fmt.Errorf("alarm: device %d is a cs463 gpo but transport is %q, want mqtt", dev.ID, dev.Transport)
	}

	if dev.Transport != outputdevice.TransportMQTT {
		return d.http.Set(ctx, dev.BaseURL, dev.SwitchID, on, offAfterSec)
	}

	if dev.Type == outputdevice.TypeCS463GPO {
		if d.gpo == nil {
			return fmt.Errorf("alarm: device %d is a cs463 gpo but reader control is not configured", dev.ID)
		}
		// TRA-1028: the reader is addressed solely by the scan_device_id FK,
		// resolved server-side into ReaderBaseTopic — never by the free-text
		// command_topic, which one org could otherwise repoint at another
		// org's reader. An unresolved topic means the reader was deleted or
		// the FK is cross-org (RLS made the join return empty); refuse to fire.
		if dev.ReaderBaseTopic == "" {
			return fmt.Errorf("alarm: device %d is a cs463 gpo but its reader base topic is unresolved (reader missing or cross-org?)", dev.ID)
		}
		// Defense in depth: the handler validates the port on write, but a row
		// predating that validation must error rather than fire the wrong port.
		if dev.SwitchID < gpoPortMin || dev.SwitchID > gpoPortMax {
			return fmt.Errorf("alarm: device %d has gpo port %d, want %d-%d", dev.ID, dev.SwitchID, gpoPortMin, gpoPortMax)
		}
		pulseMs := 0
		if on && offAfterSec > 0 {
			pulseMs = offAfterSec * 1000
		}
		return d.gpo.GpoSet(ctx, dev.ReaderBaseTopic, dev.SwitchID, on, pulseMs)
	}

	if dev.CommandTopic == nil || *dev.CommandTopic == "" {
		return fmt.Errorf("alarm: device %d uses mqtt transport but has no command_topic", dev.ID)
	}

	if d.mqtt == nil {
		return fmt.Errorf("alarm: device %d uses mqtt transport but the broker is not configured", dev.ID)
	}
	return d.mqtt.Publish(ctx, *dev.CommandTopic, dev.SwitchID, on, offAfterSec)
}
