package readerd

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"

	"github.com/trakrf/platform/mqtt-rpc/internal/readerrpc"
)

const (
	connectRetryInterval = 5 * time.Second
	publishTimeout       = 5 * time.Second
	rpcTimeout           = 10 * time.Second
	// safeStateTimeout bounds the start and stop output sweeps (four ports,
	// each bounded again by the adapter).
	safeStateTimeout = 15 * time.Second
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

	updater Updater       // optional remote self-update (SetUpdater)
	stopCh  chan struct{} // closed by Stop

	started     time.Time     // process start, for the commit uptime floor
	commitAfter time.Duration // minimum uptime before a probation commit

	// publishRetained is the retained-status seam, set by Start (status.go).
	publishRetained func(topic string, payload []byte) error

	// host runs Reader.Reboot and Reader.Shutdown (SetHost); nil disables them.
	// after schedules the delayed power action; a seam for tests.
	host       Host
	powerDelay time.Duration
	after      func(time.Duration, func())

	statusMu     sync.Mutex
	health       *readerrpc.Health // latest from the health monitor
	reason       string            // set by GoingDown before a planned reboot or power-off
	powerPending string            // a Reader.Reboot/Shutdown accepted and not yet acted on

	anchor anchorCheck // the last broker handshake, judged as CSL would
}

// CloudServerFault is the golden CloudServer fault for the retained health:
// CheckCloudServer's static findings, then whether the broker's chain reached
// the bundle's first certificate on the last handshake.
func (d *Daemon) CloudServerFault(src *CloudServerSource, now time.Time) string {
	var faults []string
	for _, f := range []string{CloudServerFault(src, now), d.anchor.Fault()} {
		if f != "" {
			faults = append(faults, f)
		}
	}
	return strings.Join(faults, "; ")
}

// New builds a Daemon. It does not connect; call Start.
func New(cfg Config, adapter Adapter, log *zerolog.Logger) *Daemon {
	return &Daemon{
		cfg:     cfg,
		adapter: adapter,
		log:     log.With().Str("component", "readerd").Logger(),
		now:     time.Now,
		stopCh:  make(chan struct{}),

		started:     time.Now(),
		commitAfter: DefaultCommitAfter,

		powerDelay: DefaultPowerDelay,
		after:      func(wait time.Duration, f func()) { time.AfterFunc(wait, f) },
	}
}

// Start connects to the broker in the background and returns immediately. The
// connection self-heals via AutoReconnect + ConnectRetry; OnConnect publishes the
// retained online status and (re)subscribes the RPC topic.
func (d *Daemon) Start() error {
	// Before serving anything: clear an output left on by a crash, a kill -9 or
	// the previous version. Not final; the daemon goes on to serve Gpo.Set.
	d.outputsOff(false)

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
		SetBinaryWill(statusTopic, lastWillPayload(), 1, true).
		SetOnConnectHandler(func(c mqtt.Client) {
			if tok := c.Publish(statusTopic, 1, true, d.onlinePayload()); tok.WaitTimeout(publishTimeout) && tok.Error() != nil {
				d.log.Error().Err(tok.Error()).Str("topic", statusTopic).Msg("publish online status failed")
			}
			if tok := c.Subscribe(rpcTopic, 1, d.handleMessage); tok.Wait() && tok.Error() != nil {
				d.log.Error().Err(tok.Error()).Str("topic", rpcTopic).Msg("subscribe rpc failed")
				return
			}
			d.log.Info().Str("rpc_topic", rpcTopic).Str("status_topic", statusTopic).Msg("connected; rpc subscribed, online published")
			if d.updater != nil && d.updater.OnProbation() {
				go func() {
					if err := d.selfTest(); err != nil {
						d.log.Warn().Err(err).Msg("update self-test publish failed")
					}
				}()
			}
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			d.log.Warn().Err(err).Msg("mqtt connection lost; auto-reconnecting")
		})

	if d.cfg.Broker.CACertPath != "" {
		opts.SetTLSConfig(brokerTLS(d.cfg.Broker.CACertPath, &d.log, &d.anchor))
	}

	d.client = mqtt.NewClient(opts)
	d.publish = func(topic string, payload []byte) error {
		tok := d.client.Publish(topic, 1, false, payload)
		if !tok.WaitTimeout(publishTimeout) {
			return fmt.Errorf("readerd: publish to %s timed out (broker unreachable?)", topic)
		}
		return tok.Error()
	}
	d.publishRetained = func(topic string, payload []byte) error {
		// IsConnectionOpen, not IsConnected: with ConnectRetry, IsConnected is
		// true while still connecting, and a publish then waits out its timeout.
		if !d.client.IsConnectionOpen() {
			return nil // OnConnect publishes the current document
		}
		tok := d.client.Publish(topic, 1, true, payload)
		if !tok.WaitTimeout(publishTimeout) {
			return fmt.Errorf("readerd: publish to %s timed out (broker unreachable?)", topic)
		}
		return tok.Error()
	}

	// Do not wait on the connect token: with ConnectRetry it only completes once a
	// connection succeeds, so waiting would hang on a down broker at boot.
	d.client.Connect()
	if d.updater != nil && d.updater.OnProbation() {
		go d.probationLoop()
	}
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

// Stop drives every output off, then publishes the retained offline status
// (best-effort) and disconnects. The outputs go first and do not depend on the
// broker: a shutdown is exactly when no OFF from the cloud will arrive.
func (d *Daemon) Stop() {
	select {
	case <-d.stopCh:
	default:
		close(d.stopCh)
	}
	d.outputsOff(true)
	if d.client == nil || !d.client.IsConnected() {
		return
	}
	statusTopic := readerrpc.StatusTopic(d.cfg.Broker.BaseTopic)
	if tok := d.client.Publish(statusTopic, 1, true, d.offlinePayload()); tok.WaitTimeout(publishTimeout) && tok.Error() != nil {
		d.log.Warn().Err(tok.Error()).Msg("publish offline status failed")
	}
	d.client.Disconnect(250)
	d.log.Info().Msg("readerd disconnected")
}

// brokerTLS trusts the CloudServer's bundle at path, as the reader's own app
// does, and falls back to the system roots when the bundle is unusable rather
// than failing. The daemon is the only remote path to the reader: refusing to
// start over a bundle that has already silenced the reader's reads would cut
// that path too, and would hide the fault the health status exists to report.
//
// With a usable bundle, every handshake is also judged against the bundle's
// first certificate alone and recorded in anchor (nil records nothing). That
// judgement never fails the daemon's own handshake.
func brokerTLS(path string, log *zerolog.Logger, anchor *anchorCheck) *tls.Config {
	cfg, err := loadTLS(path)
	if err != nil {
		log.Error().Err(err).Str("ca_cert", path).Msg("cannot use the CloudServer trust bundle; trusting the system roots instead")
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if pemData, err := os.ReadFile(path); err == nil && anchor != nil {
		if first := firstCertificate(pemData); first != nil {
			cfg.VerifyConnection = func(cs tls.ConnectionState) error {
				anchor.observe(first, cs.PeerCertificates, time.Now())
				return nil
			}
		}
	}
	return cfg
}

// outputsOff runs the adapter's output safe state. A failure is already logged
// per port by the adapter; it never stops a start or a stop.
func (d *Daemon) outputsOff(final bool) {
	ctx, cancel := context.WithTimeout(context.Background(), safeStateTimeout)
	defer cancel()
	if err := d.adapter.OutputsOff(ctx, final); err != nil {
		d.log.Error().Err(err).Bool("final", final).Msg("outputs safe state incomplete; an output may still be on")
	}
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
//
// Requests are dispatched here, on the callback, so they are driven in arrival
// order — an ON then OFF for one GPO port must never swap. The reply is published
// from its own goroutine: paho cannot process the broker's ack for a QoS 1 publish
// until this callback returns, so waiting on it here costs the full publishTimeout
// on every request and holds every later command behind it.
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
	go func() {
		if err := d.publish(replyTopic, reply); err != nil {
			d.log.Error().Err(err).Str("reply_topic", replyTopic).Msg("publish rpc reply failed")
			return
		}
		d.answered()
	}()
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
		return d.result(req, d.withPowerMethods(d.withDaemonInfo(caps)))

	case readerrpc.MethodDaemonUpdate:
		return d.daemonUpdate(req)

	case readerrpc.MethodReboot:
		return d.power(req, readerrpc.StatusReasonReboot)

	case readerrpc.MethodShutdown:
		return d.power(req, readerrpc.StatusReasonShutdown)

	case readerrpc.MethodGetOperProfile:
		var p readerrpc.OperProfileParams
		if len(req.Params) > 0 {
			if err := json.Unmarshal(req.Params, &p); err != nil {
				return readerrpc.NewError(req, readerrpc.CodeInvalidParams, "invalid GetOperProfile params: "+err.Error())
			}
		}
		cfg, err := d.adapter.GetOperProfile(ctx, p.Force)
		if err != nil {
			return d.errorResponse(req, err)
		}
		return d.result(req, cfg)

	case readerrpc.MethodSetOperProfile:
		var p readerrpc.SetOperProfileParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return readerrpc.NewError(req, readerrpc.CodeInvalidParams, "invalid SetOperProfile params: "+err.Error())
		}
		res, err := d.adapter.SetOperProfile(ctx, p.ReaderConfig, p.Force)
		if err != nil {
			return d.errorResponse(req, err)
		}
		return d.result(req, res)

	case readerrpc.MethodGpoSet:
		var p readerrpc.GpoSetParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return readerrpc.NewError(req, readerrpc.CodeInvalidParams, "invalid Gpo.Set params: "+err.Error())
		}
		if err := d.adapter.GpoSet(ctx, p.Port, p.On, p.PulseMs); err != nil {
			return d.errorResponse(req, err)
		}
		return d.result(req, readerrpc.GpoSetResult{
			Port:   p.Port,
			On:     p.On,
			Pulsed: p.On, // every ON arms a release; none latches
		})

	case readerrpc.MethodGetStatus:
		st, err := d.adapter.GetStatus(ctx)
		if err != nil {
			return readerrpc.NewError(req, readerrpc.CodeInternal, err.Error())
		}
		return d.result(req, st)

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

// errorResponse maps an adapter error to a response frame: a *readerrpc.BusyError
// becomes a typed CodeReaderBusy frame (holder IP in data); everything else is a
// generic internal error.
func (d *Daemon) errorResponse(req readerrpc.Request, err error) readerrpc.Response {
	var be *readerrpc.BusyError
	if errors.As(err, &be) {
		return readerrpc.NewBusyError(req, be.HeldBy)
	}
	return readerrpc.NewError(req, readerrpc.CodeInternal, err.Error())
}
