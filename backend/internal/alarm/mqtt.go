package alarm

import (
	"context"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"
	"github.com/trakrf/platform/backend/internal/ingest"
)

const publishTimeout = 5 * time.Second

// MQTTPublisher fires an alarm device by publishing a Shelly native MQTT control
// command to the shared broker (TRA-906): payload "on"/"off" to
// <commandTopic>/command/switch:<switchID>. The Shelly, subscribed under that
// prefix, actuates — no inbound connection to the device is needed.
type MQTTPublisher struct {
	// publish is the transport seam (topic, payload) -> error; the real impl
	// wraps a paho client, tests inject a fake.
	publish func(topic string, payload []byte) error
}

// Publish sends the on/off command for one relay channel. It does not wait for
// device acknowledgement (publish-and-trust): success means the broker accepted
// the message, not that the relay confirmed (MQTT is fire-and-forget).
func (p *MQTTPublisher) Publish(_ context.Context, commandTopic string, switchID int, on bool) error {
	topic := fmt.Sprintf("%s/command/switch:%d", commandTopic, switchID)
	payload := "off"
	if on {
		payload = "on"
	}
	return p.publish(topic, []byte(payload))
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
