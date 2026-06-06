package alarm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"
	"github.com/trakrf/platform/backend/internal/ingest"
)

const publishTimeout = 5 * time.Second

// MQTTPublisher fires an output device by publishing a Shelly Gen2+ RPC frame to
// the shared broker (TRA-906/TRA-934): a Switch.Set request on <commandTopic>/rpc.
// The Shelly, subscribed there, actuates — no inbound connection to the device is
// needed. RPC (not the plain <commandTopic>/command/switch:<id> on/off interface)
// is used so the frame can carry toggle_after, the device-side one-shot off timer.
type MQTTPublisher struct {
	// publish is the transport seam (topic, payload) -> error; the real impl
	// wraps a paho client, tests inject a fake.
	publish func(topic string, payload []byte) error
}

// Publish drives one relay channel via a Switch.Set RPC frame. When on and
// offAfterSec > 0 the frame includes toggle_after, so the device turns on and
// flips itself off after the delay (survives a backend restart; no second
// message). offAfterSec is omitted for off commands and when 0 (stay on until an
// explicit off). No "src" is set: we publish-and-trust and want no reply on the
// broker. Success means the broker accepted the message, not that the relay
// confirmed (MQTT is fire-and-forget).
func (p *MQTTPublisher) Publish(_ context.Context, commandTopic string, switchID int, on bool, offAfterSec int) error {
	params := map[string]any{"id": switchID, "on": on}
	if on && offAfterSec > 0 {
		params["toggle_after"] = offAfterSec
	}
	frame, err := json.Marshal(map[string]any{
		"id":     1,
		"method": "Switch.Set",
		"params": params,
	})
	if err != nil {
		return fmt.Errorf("alarm: marshal rpc frame: %w", err)
	}
	return p.publish(commandTopic+"/rpc", frame)
}

// NewMQTTPublisher connects a dedicated publish client to the broker described
// by cfg (reusing the ingest MQTT config / MQTT_URL) and returns the publisher
// plus a stop func. Connection is async and self-healing, mirroring the ingest
// subscriber; a down broker at boot does not block startup (publish then errors
// until connected).
func NewMQTTPublisher(cfg ingest.Config, log *zerolog.Logger) (*MQTTPublisher, func()) {
	l := log.With().Str("component", "alarm-mqtt").Logger()

	clientID := cfg.ClientID + "-pub"
	if host, _ := os.Hostname(); host != "" {
		clientID = clientID + "-" + host
	}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.URL).
		SetClientID(clientID).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			l.Warn().Err(err).Msg("mqtt publish connection lost; auto-reconnecting")
		})

	client := mqtt.NewClient(opts)
	client.Connect() // do not wait: ConnectRetry token only completes on success
	l.Info().Str("client_id", clientID).Msg("mqtt publisher connecting")

	p := &MQTTPublisher{
		publish: func(topic string, payload []byte) error {
			tok := client.Publish(topic, 1, false, payload)
			if !tok.WaitTimeout(publishTimeout) {
				return fmt.Errorf("mqtt: publish to %s timed out (broker unreachable?)", topic)
			}
			return tok.Error()
		},
	}
	stop := func() { client.Disconnect(250) }
	return p, stop
}
