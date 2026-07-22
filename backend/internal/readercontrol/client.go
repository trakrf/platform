// Package readercontrol is the cloud side of the reader MQTT JSON-RPC control
// contract (TRA-993). It drives the on-reader daemon, which subscribes to
// <base>/rpc and publishes its reply to the request's "src" topic. This client
// owns a dedicated broker connection, correlates replies to in-flight requests
// by id, and exposes typed wrappers over the readerrpc contract methods.
//
// A nil *Client means reader control is disabled (no broker configured); the
// reader-config handler reports a clear 503 in that case.
package readercontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/ingest"
	"github.com/trakrf/platform/backend/internal/readerrpc"
)

const (
	publishTimeout = 5 * time.Second
	defaultTimeout = 10 * time.Second
)

// Client is the cloud RPC client. It owns one paho client used to publish
// requests and receive replies on the cloud reply wildcard. Replies are matched
// to in-flight requests by id via the pending map.
type Client struct {
	log      zerolog.Logger
	instance string // unique per cloud client; the reply-topic namespace we own
	timeout  time.Duration

	// publish is the transport seam (topic, payload) -> error. The real impl
	// wraps the paho client; tests inject a fake to drive call without a broker.
	publish func(topic string, payload []byte) error

	mu      sync.Mutex
	nextID  int
	pending map[int]chan readerrpc.Response
}

// replyTopic is the wildcard the client subscribes to and the prefix of every
// request's src. The daemon replies to <replyBase>/<id>; we own this namespace.
func (c *Client) replyBase() string { return "trakrf-cloud/" + c.instance + "/reply" }

// New connects a dedicated paho client to the broker described by cfg (reuses
// MQTT_URL) and returns the client plus a stop func. Connection is async and
// self-healing: a down broker at boot does not block startup (calls then time
// out until connected). On connect it subscribes to the cloud reply wildcard so
// daemon replies route back here. The instance id is derived from the hostname
// so concurrent replicas own distinct reply namespaces.
func New(cfg ingest.Config, log *zerolog.Logger) (*Client, func()) {
	l := log.With().Str("component", "readercontrol").Logger()

	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}

	c := &Client{
		log:      l,
		instance: host,
		timeout:  defaultTimeout,
		pending:  make(map[int]chan readerrpc.Response),
	}

	clientID := cfg.ClientID + "-readerrpc-" + host
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.URL).
		SetClientID(clientID).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOnConnectHandler(func(cl mqtt.Client) {
			// Subscribe to our reply namespace on every (re)connect.
			wildcard := c.replyBase() + "/+"
			if tok := cl.Subscribe(wildcard, 1, func(_ mqtt.Client, m mqtt.Message) {
				c.handleReply(m.Topic(), m.Payload())
			}); tok.Wait() && tok.Error() != nil {
				l.Error().Err(tok.Error()).Str("topic", wildcard).Msg("reply subscribe failed")
				return
			}
			l.Info().Str("topic", wildcard).Msg("subscribed to reply topic")
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			l.Warn().Err(err).Msg("mqtt connection lost; auto-reconnecting")
		})

	client := mqtt.NewClient(opts)
	c.publish = func(topic string, payload []byte) error {
		tok := client.Publish(topic, 1, false, payload)
		if !tok.WaitTimeout(publishTimeout) {
			return fmt.Errorf("readercontrol: publish to %s timed out (broker unreachable?)", topic)
		}
		return tok.Error()
	}
	client.Connect() // do not wait: ConnectRetry token completes only on success
	l.Info().Str("client_id", clientID).Msg("reader-control rpc client connecting")

	return c, func() { client.Disconnect(250) }
}

// handleReply is the paho message callback. It recovers from panics so one bad
// reply never kills the client, then routes the payload through deliver.
func (c *Client) handleReply(topic string, payload []byte) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error().Interface("panic", r).Str("topic", topic).Msg("recovered from panic in reply handler")
		}
	}()
	c.deliver(payload)
}

// deliver decodes a reply frame and hands it to the waiting caller, if any. It
// is the testable seam: a unit test calls it directly to simulate the daemon's
// reply. An unknown id (timed-out or already-served call) is dropped.
func (c *Client) deliver(payload []byte) {
	var resp readerrpc.Response
	if err := json.Unmarshal(payload, &resp); err != nil {
		c.log.Error().Err(err).Msg("bad rpc reply payload")
		return
	}
	c.mu.Lock()
	ch, ok := c.pending[resp.ID]
	if ok {
		delete(c.pending, resp.ID)
	}
	c.mu.Unlock()
	if !ok {
		c.log.Debug().Int("id", resp.ID).Msg("reply for unknown/expired request; dropping")
		return
	}
	ch <- resp
}

// next returns the next monotonic request id.
func (c *Client) next() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	return c.nextID
}

// call publishes a request to the reader's <base>/rpc topic and blocks until the
// matching reply arrives, the context is canceled, or the configured timeout
// elapses. On timeout the pending entry is reclaimed and an offline-style error
// is returned. params is marshaled to the frame; pass nil for no params.
func (c *Client) call(ctx context.Context, base, method string, params any) (readerrpc.Response, error) {
	id := c.next()
	src := fmt.Sprintf("%s/%d", c.replyBase(), id)

	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return readerrpc.Response{}, fmt.Errorf("readercontrol: marshal params: %w", err)
		}
		raw = b
	}

	req := readerrpc.Request{ID: id, Src: src, Method: method, Params: raw}
	frame, err := json.Marshal(req)
	if err != nil {
		return readerrpc.Response{}, fmt.Errorf("readercontrol: marshal request: %w", err)
	}

	ch := make(chan readerrpc.Response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.publish(readerrpc.RPCTopic(base), frame); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return readerrpc.Response{}, fmt.Errorf("readercontrol: publish request: %w", err)
	}

	timer := time.NewTimer(c.timeout)
	defer timer.Stop()

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return readerrpc.Response{}, fmt.Errorf("readercontrol: %s on %s: reader did not respond (offline?): %w", method, base, ctx.Err())
	case <-timer.C:
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return readerrpc.Response{}, fmt.Errorf("readercontrol: %s on %s: reader did not respond (offline?)", method, base)
	}
}

// Notify publishes a request frame and returns as soon as the broker accepts
// it, without registering a pending entry or awaiting a reply. It is the
// fire-and-forget counterpart to call, for paths where blocking on a reader
// round trip is worse than not knowing the outcome — the alarm fire path
// (TRA-1028), where a slow-but-alive reader must not delay an alarm.
//
// The frame still carries a real id and src, so the daemon behaves identically
// and needs no change. Its reply lands on our subscribed wildcard and deliver
// drops it as an unknown id, which is the intended outcome.
//
// Success means the broker accepted the message, not that the reader actuated.
func (c *Client) Notify(_ context.Context, base, method string, params any) error {
	id := c.next()

	var raw json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("readercontrol: marshal params: %w", err)
		}
		raw = b
	}

	req := readerrpc.Request{ID: id, Src: fmt.Sprintf("%s/%d", c.replyBase(), id), Method: method, Params: raw}
	frame, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("readercontrol: marshal request: %w", err)
	}
	if err := c.publish(readerrpc.RPCTopic(base), frame); err != nil {
		return fmt.Errorf("readercontrol: publish %s: %w", method, err)
	}
	return nil
}

// rpcErr converts a frame's error object to a Go error. A CodeReaderBusy frame
// becomes a typed *readerrpc.BusyError so the HTTP layer can map it to a 409.
func rpcErr(method string, e *readerrpc.RPCError) error {
	if e.Code == readerrpc.CodeReaderBusy {
		var d readerrpc.ReaderBusyData
		_ = json.Unmarshal(e.Data, &d)
		return &readerrpc.BusyError{HeldBy: d.HeldBy}
	}
	return fmt.Errorf("readercontrol: %s: reader error %d: %s", method, e.Code, e.Message)
}

// GetCapabilities asks the reader what it supports.
func (c *Client) GetCapabilities(ctx context.Context, base string) (readerrpc.Capabilities, error) {
	resp, err := c.call(ctx, base, readerrpc.MethodGetCapabilities, nil)
	if err != nil {
		return readerrpc.Capabilities{}, err
	}
	if resp.Error != nil {
		return readerrpc.Capabilities{}, rpcErr(readerrpc.MethodGetCapabilities, resp.Error)
	}
	var caps readerrpc.Capabilities
	if err := json.Unmarshal(resp.Result, &caps); err != nil {
		return readerrpc.Capabilities{}, fmt.Errorf("readercontrol: decode capabilities: %w", err)
	}
	return caps, nil
}

// GetOperProfile reads the reader's current configuration. force force-logs-out a
// held single session first.
func (c *Client) GetOperProfile(ctx context.Context, base string, force bool) (readerrpc.ReaderConfig, error) {
	resp, err := c.call(ctx, base, readerrpc.MethodGetOperProfile, readerrpc.OperProfileParams{Force: force})
	if err != nil {
		return readerrpc.ReaderConfig{}, err
	}
	if resp.Error != nil {
		return readerrpc.ReaderConfig{}, rpcErr(readerrpc.MethodGetOperProfile, resp.Error)
	}
	var cfg readerrpc.ReaderConfig
	if err := json.Unmarshal(resp.Result, &cfg); err != nil {
		return readerrpc.ReaderConfig{}, fmt.Errorf("readercontrol: decode config: %w", err)
	}
	return cfg, nil
}

// SetOperProfile applies a (partial) configuration to the reader. force
// force-logs-out a held single session first.
func (c *Client) SetOperProfile(ctx context.Context, base string, cfg readerrpc.ReaderConfig, force bool) (readerrpc.SetConfigResult, error) {
	resp, err := c.call(ctx, base, readerrpc.MethodSetOperProfile, readerrpc.SetOperProfileParams{ReaderConfig: cfg, Force: force})
	if err != nil {
		return readerrpc.SetConfigResult{}, err
	}
	if resp.Error != nil {
		return readerrpc.SetConfigResult{}, rpcErr(readerrpc.MethodSetOperProfile, resp.Error)
	}
	var res readerrpc.SetConfigResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return readerrpc.SetConfigResult{}, fmt.Errorf("readercontrol: decode set-config result: %w", err)
	}
	return res, nil
}

// GpoSet drives one general purpose output on the reader. It is fire-and-forget
// (see Notify): it returns once the broker accepts the frame, and never waits
// for the reader to confirm. pulseMs > 0 with on=true arms a reader-side
// one-shot, the analog of the Shelly device-side toggle_after, so the OFF edge
// does not depend on a second message arriving.
func (c *Client) GpoSet(ctx context.Context, base string, port int, on bool, pulseMs int) error {
	return c.Notify(ctx, base, readerrpc.MethodGpoSet, readerrpc.GpoSetParams{
		Port:    port,
		On:      on,
		PulseMs: pulseMs,
	})
}
