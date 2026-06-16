package readerd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

const (
	connectRetryInterval = 5 * time.Second
	publishTimeout       = 5 * time.Second
	rpcTimeout           = 10 * time.Second
)

// Daemon is the on-reader MQTT JSON-RPC control endpoint. It subscribes to the
// reader's RPC topic, dispatches each request to the reader-agnostic Adapter, and
// publishes the response back to the request's reply (Src) topic. It also keeps a
// retained online/offline presence flag on the status topic via an MQTT Last Will.
type Daemon struct {
	cfg     Config
	adapter Adapter
	client  mqtt.Client
	log     zerolog.Logger
	now     func() time.Time

	// publish is the transport seam (topic, payload) -> error; the real impl wraps
	// the paho client, tests can inject a fake.
	publish func(topic string, payload []byte) error
}

// New builds a Daemon. It does not connect; call Start.
func New(cfg Config, adapter Adapter, log *zerolog.Logger) *Daemon {
	return &Daemon{
		cfg:     cfg,
		adapter: adapter,
		log:     log.With().Str("component", "readerd").Logger(),
		now:     time.Now,
	}
}

// statusPayload is the retained presence document published on the status topic.
type statusPayload struct {
	Online bool   `json:"online"`
	At     string `json:"at,omitempty"`
}

func (d *Daemon) onlinePayload() []byte {
	b, _ := json.Marshal(statusPayload{Online: true, At: d.now().UTC().Format(time.RFC3339)})
	return b
}

func offlinePayload() []byte {
	b, _ := json.Marshal(statusPayload{Online: false})
	return b
}

// Start connects to the broker in the background and returns immediately. The
// connection self-heals via AutoReconnect + ConnectRetry; OnConnect publishes the
// retained online status and (re)subscribes the RPC topic.
func (d *Daemon) Start() error {
	statusTopic := readerrpc.StatusTopic(d.cfg.Broker.BaseTopic)
	rpcTopic := readerrpc.RPCTopic(d.cfg.Broker.BaseTopic)

	opts := mqtt.NewClientOptions().
		AddBroker(d.cfg.Broker.URL).
		SetClientID(d.cfg.Broker.RPCClientID). // MUST differ from the reader's own clientId
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(connectRetryInterval).
		// Last Will: if the daemon drops without a clean Stop, the broker publishes
		// the retained offline document so the cloud sees the reader go away.
		SetBinaryWill(statusTopic, offlinePayload(), 1, true).
		SetOnConnectHandler(func(c mqtt.Client) {
			if tok := c.Publish(statusTopic, 1, true, d.onlinePayload()); tok.WaitTimeout(publishTimeout) && tok.Error() != nil {
				d.log.Error().Err(tok.Error()).Str("topic", statusTopic).Msg("publish online status failed")
			}
			if tok := c.Subscribe(rpcTopic, 1, d.handleMessage); tok.Wait() && tok.Error() != nil {
				d.log.Error().Err(tok.Error()).Str("topic", rpcTopic).Msg("subscribe rpc failed")
				return
			}
			d.log.Info().Str("rpc_topic", rpcTopic).Str("status_topic", statusTopic).Msg("connected; rpc subscribed, online published")
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			d.log.Warn().Err(err).Msg("mqtt connection lost; auto-reconnecting")
		})

	if d.cfg.Broker.CACertPath != "" {
		tlsCfg, err := loadTLS(d.cfg.Broker.CACertPath)
		if err != nil {
			return fmt.Errorf("readerd: load CA cert: %w", err)
		}
		opts.SetTLSConfig(tlsCfg)
	}

	d.client = mqtt.NewClient(opts)
	d.publish = func(topic string, payload []byte) error {
		tok := d.client.Publish(topic, 1, false, payload)
		if !tok.WaitTimeout(publishTimeout) {
			return fmt.Errorf("readerd: publish to %s timed out (broker unreachable?)", topic)
		}
		return tok.Error()
	}

	// Do not wait on the connect token: with ConnectRetry it only completes once a
	// connection succeeds, so waiting would hang on a down broker at boot.
	d.client.Connect()
	d.log.Info().Str("client_id", d.cfg.Broker.RPCClientID).Str("broker", redactBrokerURL(d.cfg.Broker.URL)).Msg("readerd connecting")
	return nil
}

// redactBrokerURL masks the password in a broker URL so credentials never reach
// the logs (the discovered CloudServer URL carries user:pass). User and host are
// preserved for diagnostics; an unparseable URL is reported as "<redacted>".
func redactBrokerURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<redacted>"
	}
	if u.User != nil {
		if _, hasPass := u.User.Password(); hasPass {
			u.User = url.UserPassword(u.User.Username(), "xxxxx")
		}
	}
	return u.String()
}

// Stop publishes the retained offline status (best-effort) then disconnects.
func (d *Daemon) Stop() {
	if d.client == nil || !d.client.IsConnected() {
		return
	}
	statusTopic := readerrpc.StatusTopic(d.cfg.Broker.BaseTopic)
	if tok := d.client.Publish(statusTopic, 1, true, offlinePayload()); tok.WaitTimeout(publishTimeout) && tok.Error() != nil {
		d.log.Warn().Err(tok.Error()).Msg("publish offline status failed")
	}
	d.client.Disconnect(250)
	d.log.Info().Msg("readerd disconnected")
}

// loadTLS builds a TLS config trusting the CA bundle at path.
func loadTLS(path string) (*tls.Config, error) {
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no certificates parsed from %s", path)
	}
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, nil
}

// handleMessage is the paho callback. It recovers from panics so one bad request
// never kills the daemon, dispatches via handleRPC, and publishes any reply to the
// request's reply topic.
func (d *Daemon) handleMessage(_ mqtt.Client, m mqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			d.log.Error().Interface("panic", r).Str("topic", m.Topic()).Msg("recovered from panic in rpc handler")
		}
	}()

	replyTopic, reply := d.handleRPC(m.Payload())
	if replyTopic == "" || reply == nil {
		return
	}
	if err := d.publish(replyTopic, reply); err != nil {
		d.log.Error().Err(err).Str("reply_topic", replyTopic).Msg("publish rpc reply failed")
	}
}

// handleRPC is the testable dispatch core. It parses the request, drives the
// adapter, and returns the reply topic (the request's Src) plus the marshaled
// response. An unparseable frame cannot be routed (no Src) so it is dropped:
// replyTopic == "" and reply == nil.
func (d *Daemon) handleRPC(payload []byte) (replyTopic string, reply []byte) {
	req, err := readerrpc.ParseRequest(payload)
	if err != nil {
		d.log.Error().Err(err).Msg("unparseable rpc request; dropping (no reply topic)")
		return "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	resp := d.dispatch(ctx, req)

	out, err := resp.Marshal()
	if err != nil {
		d.log.Error().Err(err).Int("id", req.ID).Msg("marshal rpc response failed")
		fallback := readerrpc.NewError(req, readerrpc.CodeInternal, "internal marshal error")
		out, _ = fallback.Marshal()
	}
	return req.Src, out
}

// dispatch routes one request to the adapter and builds the response frame.
func (d *Daemon) dispatch(ctx context.Context, req readerrpc.Request) readerrpc.Response {
	switch req.Method {
	case readerrpc.MethodGetCapabilities:
		caps, err := d.adapter.GetCapabilities(ctx)
		if err != nil {
			return readerrpc.NewError(req, readerrpc.CodeInternal, err.Error())
		}
		return d.result(req, caps)

	case readerrpc.MethodGetConfig:
		cfg, err := d.adapter.GetConfig(ctx)
		if err != nil {
			return readerrpc.NewError(req, readerrpc.CodeInternal, err.Error())
		}
		return d.result(req, cfg)

	case readerrpc.MethodGetStatus:
		st, err := d.adapter.GetStatus(ctx)
		if err != nil {
			return readerrpc.NewError(req, readerrpc.CodeInternal, err.Error())
		}
		return d.result(req, st)

	case readerrpc.MethodSetConfig:
		var cfg readerrpc.ReaderConfig
		if err := json.Unmarshal(req.Params, &cfg); err != nil {
			return readerrpc.NewError(req, readerrpc.CodeInvalidParams, "invalid SetConfig params: "+err.Error())
		}
		res, err := d.adapter.SetConfig(ctx, cfg)
		if err != nil {
			return readerrpc.NewError(req, readerrpc.CodeInternal, err.Error())
		}
		return d.result(req, res)

	default:
		return readerrpc.NewError(req, readerrpc.CodeMethodNotFound, "unsupported method: "+req.Method)
	}
}

// result wraps NewResult, converting a marshal failure into an internal error frame.
func (d *Daemon) result(req readerrpc.Request, v any) readerrpc.Response {
	resp, err := readerrpc.NewResult(req, v)
	if err != nil {
		return readerrpc.NewError(req, readerrpc.CodeInternal, "marshal result: "+err.Error())
	}
	return resp
}
