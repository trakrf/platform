// Package readercontrol is the backend seam to the CS463 power agent (TRA-993).
// It publishes power commands to {publish_topic}/command/power and subscribes to
// {publish_topic}/state/power, writing confirmed results back to scan_point
// metadata. Mirrors the alarm http/mqtt Dispatcher pattern: the broker is the
// transport, the agent does the reader work. nil Controller means MQTT is
// disabled (the handler then reports a clear error).
package readercontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/ingest"
	"github.com/trakrf/platform/backend/internal/readerpower"
	"github.com/trakrf/platform/backend/internal/storage"
)

const publishTimeout = 5 * time.Second

// RouteLookup resolves a reader's publish_topic to its org/device and lists the
// registered reader topics. *topicroute.Registry satisfies it.
type RouteLookup interface {
	Lookup(topic string) (storage.ScanRoute, bool)
	Topics() []string
}

// StateStore persists agent state. *storage.Storage satisfies it.
type StateStore interface {
	SetAntennaPowerState(ctx context.Context, route storage.ScanRoute, st readerpower.State) error
}

// Controller owns one broker client used for both command publish and state
// subscribe.
type Controller struct {
	client  mqtt.Client
	publish func(topic string, payload []byte) error
	store   StateStore
	routes  RouteLookup
	log     zerolog.Logger
}

// PublishPowerCommand publishes a power command for the reader identified by
// publishTopic.
func (c *Controller) PublishPowerCommand(_ context.Context, publishTopic string, cmd readerpower.Command) error {
	payload, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("readercontrol: marshal command: %w", err)
	}
	return c.publish(readerpower.CommandTopic(publishTopic), payload)
}

// New connects a Controller to the broker described by cfg (reuses MQTT_URL). It
// is async/self-healing: a broker down at boot does not block startup. Returns
// the controller and a stop func.
func New(cfg ingest.Config, store StateStore, routes RouteLookup, log *zerolog.Logger) (*Controller, func()) {
	l := log.With().Str("component", "readercontrol").Logger()
	c := &Controller{store: store, routes: routes, log: l}

	clientID := cfg.ClientID + "-readerctl"
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
		SetOnConnectHandler(func(cl mqtt.Client) {
			c.subscribeState(cl)
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			l.Warn().Err(err).Msg("mqtt connection lost; auto-reconnecting")
		})

	client := mqtt.NewClient(opts)
	c.client = client
	c.publish = func(topic string, payload []byte) error {
		tok := client.Publish(topic, 1, false, payload)
		if !tok.WaitTimeout(publishTimeout) {
			return fmt.Errorf("readercontrol: publish to %s timed out (broker unreachable?)", topic)
		}
		return tok.Error()
	}
	client.Connect() // do not wait: ConnectRetry token completes only on success
	l.Info().Str("client_id", clientID).Msg("reader-control connecting")
	return c, func() { client.Disconnect(250) }
}

// subscribeState subscribes to the state topic of every registered reader. New
// readers added after connect are picked up on the next reconnect; the periodic
// reconcile (serve) plus reconnects keep this fresh for 993a.
func (c *Controller) subscribeState(cl mqtt.Client) {
	topics := c.routes.Topics()
	if len(topics) == 0 {
		return
	}
	filters := make(map[string]byte, len(topics))
	for _, t := range topics {
		filters[readerpower.StateTopic(t)] = 1
	}
	if tok := cl.SubscribeMultiple(filters, c.handleState); tok.Wait() && tok.Error() != nil {
		c.log.Error().Err(tok.Error()).Int("count", len(filters)).Msg("state subscribe failed")
		return
	}
	c.log.Info().Int("count", len(filters)).Msg("subscribed to reader state topics")
}

func (c *Controller) handleState(_ mqtt.Client, m mqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error().Interface("panic", r).Str("topic", m.Topic()).Msg("recovered from panic in state handler")
		}
	}()
	c.processState(m.Topic(), m.Payload())
}

// processState is the testable core of the state handler.
func (c *Controller) processState(topic string, payload []byte) {
	publishTopic := strings.TrimSuffix(topic, readerpower.StateTopic(""))
	route, ok := c.routes.Lookup(publishTopic)
	if !ok {
		c.log.Warn().Str("topic", topic).Msg("state for unknown reader; ignoring")
		return
	}
	var st readerpower.State
	if err := json.Unmarshal(payload, &st); err != nil {
		c.log.Error().Err(err).Str("topic", topic).Msg("bad state payload")
		return
	}
	if err := c.store.SetAntennaPowerState(context.Background(), route, st); err != nil {
		c.log.Error().Err(err).Str("topic", topic).Msg("persist state failed")
	}
}
