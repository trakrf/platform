package ingest

import (
	"context"
	"errors"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/models/scanread"
	"github.com/trakrf/platform/backend/internal/storage"
)

// ReadEvaluator receives the membership-passing reads of each message so a
// downstream consumer (the TRA-901 geofence engine) can act on them. Defined
// here (not in geofence) so the subscriber depends on a local interface, not the
// engine — keeping ingest free of a geofence import. *geofence.Engine satisfies it.
type ReadEvaluator interface {
	Evaluate(ctx context.Context, orgID int, tagScanID int64, receivedAt time.Time, reads []storage.ResolvedRead)
}

// ReadPublisher receives every parsed read (before membership filtering) for
// live fan-out — the org-scoped SSE Live Reads feed (TRA-924). Unlike the
// geofence path it sees ALL reads, because Live Reads is a coverage diagnostic
// that must surface unknown EPCs too. Defined here so ingest depends on a local
// interface, not the readstream service. Optional; nil disables fan-out.
type ReadPublisher interface {
	Publish(orgID int, topic string, reads []scanread.Read)
}

// Subscriber consumes MQTT reads and derives asset_scans (TRA-900). It is the
// observable replacement for the silent process_tag_scans trigger.
type Subscriber struct {
	cfg    Config
	store  *storage.Storage
	eval   ReadEvaluator // optional; nil disables geofence evaluation
	feed   ReadPublisher // optional; nil disables live-feed fan-out
	log    zerolog.Logger
	client mqtt.Client
}

// NewSubscriber builds a subscriber. It does not connect; call Start. eval and
// feed may each be nil (no geofence evaluation / no live-feed fan-out).
func NewSubscriber(cfg Config, store *storage.Storage, eval ReadEvaluator, feed ReadPublisher, log *zerolog.Logger) *Subscriber {
	return &Subscriber{cfg: cfg, store: store, eval: eval, feed: feed, log: log.With().Str("component", "ingest").Logger()}
}

// Start begins connecting in the background and returns immediately — it never
// blocks server startup on broker availability. ConnectRetry + AutoReconnect
// keep the client trying until the broker is reachable, then OnConnect
// (re)subscribes; every state change is logged. Message handling runs on paho's
// goroutines until Stop.
func (s *Subscriber) Start() error {
	clientID := s.cfg.ClientID
	if host, _ := os.Hostname(); host != "" {
		clientID = clientID + "-" + host // unique per replica; avoid duplicate-id disconnect loops
	}

	opts := mqtt.NewClientOptions().
		AddBroker(s.cfg.URL).
		SetClientID(clientID).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOnConnectHandler(func(c mqtt.Client) {
			if tok := c.Subscribe(s.cfg.Topic, 1, s.handleMessage); tok.Wait() && tok.Error() != nil {
				s.log.Error().Err(tok.Error()).Str("topic", s.cfg.Topic).Msg("subscribe failed")
				return
			}
			s.log.Info().Str("topic", s.cfg.Topic).Msg("subscribed")
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			s.log.Warn().Err(err).Msg("mqtt connection lost; auto-reconnecting")
		})

	s.client = mqtt.NewClient(opts)
	// Do NOT wait on the connect token: with ConnectRetry the token only
	// completes once a connection succeeds, so waiting would hang serve.Run
	// whenever the broker is down at boot. The connection is established
	// asynchronously and self-heals.
	s.client.Connect()
	s.log.Info().Str("client_id", clientID).Str("topic", s.cfg.Topic).Msg("mqtt subscriber connecting")
	return nil
}

// Stop disconnects the client (idempotent).
func (s *Subscriber) Stop() {
	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(250)
		s.log.Info().Msg("mqtt subscriber disconnected")
	}
}

// handleMessage is the per-message pipeline. It recovers from panics so one bad
// payload never kills ingestion, and it logs/metrics every outcome (no silent
// swallow, unlike the old trigger).
func (s *Subscriber) handleMessage(_ mqtt.Client, m mqtt.Message) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Error().Interface("panic", r).Str("topic", m.Topic()).Msg("recovered from panic in handler")
			metricMessages.WithLabelValues("panic").Inc()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	topic, payload := m.Topic(), m.Payload()
	receivedAt := time.Now() // server time wins over reader timeStampOfRead
	metricMessages.WithLabelValues("received").Inc()

	// 1. Always append to the audit log first (gives us tag_scan_id provenance).
	tagScanID, err := s.store.InsertRawTagScan(ctx, topic, payload)
	if err != nil {
		s.log.Error().Err(err).Str("topic", topic).Msg("audit insert failed")
		metricMessages.WithLabelValues("audit_error").Inc()
		return
	}

	// 2. Route topic -> org/device (SECURITY DEFINER; no org context yet).
	route, found, err := s.store.ResolveScanTopic(ctx, topic)
	if err != nil {
		s.log.Error().Err(err).Str("topic", topic).Msg("topic resolution failed")
		metricMessages.WithLabelValues("resolve_error").Inc()
		return
	}
	if !found {
		s.log.Debug().Str("topic", topic).Msg("unregistered topic; audit kept, no derivation")
		metricMessages.WithLabelValues("unregistered_topic").Inc()
		return
	}

	// 3. Parse by registered device type.
	reads, err := Parse(route.DeviceType, payload)
	if errors.Is(err, ErrUnsupportedDevice) {
		s.log.Debug().Str("topic", topic).Str("device_type", route.DeviceType).Msg("unsupported device type; deferred")
		metricMessages.WithLabelValues("unsupported_device").Inc()
		return
	}
	if err != nil {
		s.log.Error().Err(err).Str("topic", topic).Msg("parse failed")
		metricMessages.WithLabelValues("parse_error").Inc()
		return
	}
	metricReadsParsed.Add(float64(len(reads)))

	// 3b. Live-feed fan-out (TRA-924): hand the full parsed set to the org-scoped
	// SSE feed before membership filtering — Live Reads is a coverage diagnostic
	// and must surface unknown EPCs too. Best-effort and non-blocking (the
	// broadcaster drops for slow clients); never affects derivation.
	if s.feed != nil {
		s.feed.Publish(route.OrgID, topic, reads)
	}

	// 4. Derive asset_scans under org context (RLS-correct).
	// TRA-901 seam: `reads` is also where the geofence engine will be handed the
	// parsed observations for the immediate-on-entry alarm decision.
	res, err := s.store.PersistReads(ctx, route.OrgID, route.ScanDeviceID, tagScanID, receivedAt, reads)
	if err != nil {
		// The raw message is already durable in tag_scans (audit row above), so a
		// transient failure here loses only the derivation, which is reproducible
		// from the audit log. Replay/backfill is owned by the cutover work (TRA-907).
		s.log.Error().Err(err).Str("topic", topic).Int("org_id", route.OrgID).Int64("tag_scan_id", tagScanID).Msg("derivation failed")
		metricMessages.WithLabelValues("derive_error").Inc()
		return
	}
	metricAssetScansInserted.Add(float64(res.Inserted))
	for reason, n := range res.Dropped {
		metricReadsDropped.WithLabelValues(reason).Add(float64(n))
	}

	// 5. Geofence evaluation (TRA-901). Best-effort and outside the derivation
	// transaction: a slow/failed alarm path must never lose a scan. Only the
	// membership-passing reads are handed off.
	if s.eval != nil && len(res.Resolved) > 0 {
		s.eval.Evaluate(ctx, route.OrgID, tagScanID, receivedAt, res.Resolved)
	}

	s.log.Info().
		Str("topic", topic).Int("org_id", route.OrgID).
		Int("parsed", len(reads)).Int("inserted", res.Inserted).
		Interface("dropped", res.Dropped).
		Msg("ingest message processed")
}
