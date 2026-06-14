// Package poweragent is the standalone edge/reader agent that bridges MQTT power
// commands to a CS463's HTTP API. It subscribes to {publish_topic}/command/power
// for each configured reader, performs a fast login->getOperProfile->setOperProfile
// ->logout cycle (see internal/poweragent/csl), and publishes the result to
// {publish_topic}/state/power. It holds NO database connection — it is configured
// entirely from a reader list (see config.go).
package poweragent

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/poweragent/csl"
	"github.com/trakrf/platform/backend/internal/readerpower"
)

const (
	applyTimeout   = 20 * time.Second
	publishTimeout = 5 * time.Second
)

// applier is the reader-control seam; *csl.Client satisfies it. Tests inject a fake.
type applier interface {
	Apply(ctx context.Context, powers map[int]float64, force bool) (csl.Result, error)
}

type reader struct {
	cfg    ReaderConfig
	client applier
}

// Agent subscribes to reader command topics and drives the readers.
type Agent struct {
	cfg     Config
	log     zerolog.Logger
	readers map[string]*reader // keyed by command topic
	client  mqtt.Client
	now     func() time.Time
}

// New builds an Agent from cfg. One csl.Client is created per reader.
func New(cfg Config, log zerolog.Logger) *Agent {
	a := &Agent{
		cfg:     cfg,
		log:     log.With().Str("component", "poweragent").Logger(),
		readers: make(map[string]*reader, len(cfg.Readers)),
		now:     time.Now,
	}
	for _, rc := range cfg.Readers {
		a.readers[readerpower.CommandTopic(rc.PublishTopic)] = &reader{
			cfg:    rc,
			client: csl.New(rc.BaseURL, rc.Username, rc.Password, 0),
		}
	}
	return a
}

// Start connects to the broker and subscribes to every reader's command topic.
// Connection is async/self-healing (mirrors the ingest subscriber): a broker
// that is down at boot does not block startup.
func (a *Agent) Start() {
	clientID := a.cfg.ClientID
	if host, _ := os.Hostname(); host != "" {
		clientID = clientID + "-" + host
	}
	opts := mqtt.NewClientOptions().
		AddBroker(a.cfg.BrokerURL).
		SetClientID(clientID).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOnConnectHandler(func(c mqtt.Client) {
			filters := make(map[string]byte, len(a.readers))
			for topic := range a.readers {
				filters[topic] = 1
			}
			if len(filters) == 0 {
				return
			}
			if tok := c.SubscribeMultiple(filters, a.handleMessage); tok.Wait() && tok.Error() != nil {
				a.log.Error().Err(tok.Error()).Int("count", len(filters)).Msg("command subscribe failed")
				return
			}
			a.log.Info().Int("count", len(filters)).Msg("subscribed to reader command topics")
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			a.log.Warn().Err(err).Msg("mqtt connection lost; auto-reconnecting")
		})

	a.client = mqtt.NewClient(opts)
	a.client.Connect() // do not wait: ConnectRetry token completes only on success
	a.log.Info().Str("client_id", clientID).Int("readers", len(a.readers)).Msg("power agent connecting")
}

// Stop disconnects the broker client.
func (a *Agent) Stop() {
	if a.client != nil {
		a.client.Disconnect(250)
	}
}

func (a *Agent) handleMessage(_ mqtt.Client, m mqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			a.log.Error().Interface("panic", r).Str("topic", m.Topic()).Msg("recovered from panic in command handler")
		}
	}()
	rdr, ok := a.readers[m.Topic()]
	if !ok {
		a.log.Warn().Str("topic", m.Topic()).Msg("command for unknown reader topic; ignoring")
		return
	}
	var cmd readerpower.Command
	if err := json.Unmarshal(m.Payload(), &cmd); err != nil {
		a.log.Error().Err(err).Str("topic", m.Topic()).Msg("bad command payload")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	state := a.handleCommand(ctx, rdr, cmd)

	payload, err := json.Marshal(state)
	if err != nil {
		a.log.Error().Err(err).Msg("marshal state")
		return
	}
	stateTopic := readerpower.StateTopic(rdr.cfg.PublishTopic)
	if tok := a.client.Publish(stateTopic, 1, false, payload); tok.WaitTimeout(publishTimeout) && tok.Error() != nil {
		a.log.Error().Err(tok.Error()).Str("topic", stateTopic).Msg("publish state failed")
	}
}

// handleCommand executes one command against a reader and returns the state to
// publish. Pure given the reader's applier — unit-tested with a fake.
func (a *Agent) handleCommand(ctx context.Context, rdr *reader, cmd readerpower.Command) readerpower.State {
	state := readerpower.State{RequestID: cmd.RequestID, Timestamp: a.now().UTC().Format(time.RFC3339)}

	powers, err := toIntPowers(cmd.Powers)
	if err != nil {
		state.Status = readerpower.StatusError
		state.Error = err.Error()
		return state
	}

	res, err := rdr.client.Apply(ctx, powers, cmd.Force)
	if err != nil {
		a.log.Error().Err(err).Str("reader", rdr.cfg.PublishTopic).Msg("apply failed")
		state.Status = readerpower.StatusError
		state.Error = err.Error()
		return state
	}
	if res.Busy {
		state.Status = readerpower.StatusBusy
		state.HolderIP = res.HolderIP
		return state
	}
	state.Status = readerpower.StatusOK
	state.ActiveProfile = res.ActiveProfile
	state.Powers = fromIntPowers(res.Powers)
	return state
}

func toIntPowers(in map[string]float64) (map[int]float64, error) {
	out := make(map[int]float64, len(in))
	for k, v := range in {
		port, err := strconv.Atoi(k)
		if err != nil {
			return nil, err
		}
		out[port] = v
	}
	return out, nil
}

func fromIntPowers(in map[int]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[strconv.Itoa(k)] = v
	}
	return out
}
