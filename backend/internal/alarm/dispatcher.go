package alarm

import (
	"context"
	"fmt"

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
)

// httpSetter drives a device over local HTTP; *shelly.Client satisfies it.
type httpSetter interface {
	Set(ctx context.Context, baseURL string, switchID int, on bool, offAfterSec int) error
}

// mqttPublisher drives a device by publishing to the shared broker;
// *MQTTPublisher satisfies it. nil means MQTT is unavailable (broker disabled).
type mqttPublisher interface {
	Publish(ctx context.Context, commandTopic string, switchID int, on bool, offAfterSec int) error
}

// Dispatcher routes a fire to the right transport per device (TRA-906): http
// devices go over local HTTP, mqtt devices publish to the broker. It is the
// single Set(device, on, offAfterSec) seam used by both the geofence Firer and
// the output-device test-fire/reset handlers.
type Dispatcher struct {
	http httpSetter
	mqtt mqttPublisher // may be nil when the broker is disabled
}

// NewDispatcher builds a Dispatcher. mqtt may be nil (MQTT-transport devices
// then fail with a clear error; http devices are unaffected).
func NewDispatcher(http httpSetter, mqtt mqttPublisher) Dispatcher {
	return Dispatcher{http: http, mqtt: mqtt}
}

// Set drives the device on/off using its configured transport. offAfterSec, when
// on and > 0, is the device-side flip-back timer in seconds (Shelly toggle_after);
// 0 means it stays on until an explicit off (manual reset).
func (d Dispatcher) Set(ctx context.Context, dev outputdevice.OutputDevice, on bool, offAfterSec int) error {
	if dev.Transport == outputdevice.TransportMQTT {
		if d.mqtt == nil {
			return fmt.Errorf("alarm: device %d uses mqtt transport but the broker is not configured", dev.ID)
		}
		if dev.CommandTopic == nil || *dev.CommandTopic == "" {
			return fmt.Errorf("alarm: device %d uses mqtt transport but has no command_topic", dev.ID)
		}
		return d.mqtt.Publish(ctx, *dev.CommandTopic, dev.SwitchID, on, offAfterSec)
	}
	return d.http.Set(ctx, dev.BaseURL, dev.SwitchID, on, offAfterSec)
}
