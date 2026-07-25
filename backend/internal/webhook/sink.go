package webhook

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/trakrf/platform/backend/internal/assetevent"
	webhookmodel "github.com/trakrf/platform/backend/internal/models/webhook"
)

// sinkStore is the storage surface the sink needs; *storage.Storage satisfies
// it. The single call returns the row and the org's entitlement together, so
// the gate below costs no extra round trip.
type sinkStore interface {
	GetWebhookForDelivery(ctx context.Context, orgID int) (*webhookmodel.Webhook, bool, error)
}

// Sink delivers asset events to the org's registered endpoint. It satisfies
// assetevent.Sink.
type Sink struct {
	store  sinkStore
	client *Client
	log    zerolog.Logger
}

// NewSink builds the webhook sink.
func NewSink(store sinkStore, client *Client, log *zerolog.Logger) *Sink {
	return &Sink{
		store:  store,
		client: client,
		log:    log.With().Str("component", "webhook").Logger(),
	}
}

// Deliver resolves the org's webhook and posts the event.
//
// Three cases return nil without sending — nothing to retry:
//
//   - the org registered no webhook;
//   - the webhook exists but is disabled;
//   - the org is not entitled.
//
// The entitlement case is the one that needs stating. Delivery is an OUTBOUND
// request, so no middleware touches it: without this check an org that
// registered a webhook during a trial and never converted would keep receiving
// events indefinitely, which directly contradicts "core capability for all PAID
// customers". Skipped events are dropped, never buffered — reinstating a
// subscription must not dump a flood of stale events describing a world that
// has moved on, the same reasoning as QoS0 fire-and-forget ingestion.
func (s *Sink) Deliver(ctx context.Context, ev assetevent.AssetMoved) error {
	wh, entitled, err := s.store.GetWebhookForDelivery(ctx, ev.OrgID)
	if err != nil {
		metricLookupErrors.Inc()
		// Transient by assumption (the row is not going anywhere), so this is
		// worth a retry — unlike the skip cases below.
		return err
	}
	if wh == nil {
		metricDeliveries.WithLabelValues("skipped_no_webhook").Inc()
		return nil
	}
	if !wh.Enabled {
		metricDeliveries.WithLabelValues("skipped_disabled").Inc()
		return nil
	}
	if !entitled {
		metricDeliveries.WithLabelValues("skipped_unentitled").Inc()
		s.log.Info().
			Int("org_id", ev.OrgID).
			Str("delivery_id", ev.DeliveryID).
			Msg("webhook delivery skipped: organization is not entitled")
		return nil
	}

	start := time.Now()
	status, err := s.client.Deliver(ctx, wh.URL, wh.Secret, ev)
	metricDeliverySeconds.Observe(time.Since(start).Seconds())
	if err != nil {
		metricDeliveries.WithLabelValues("failed").Inc()
		s.log.Warn().Err(err).
			Int("org_id", ev.OrgID).
			Int("status", status).
			Str("delivery_id", ev.DeliveryID).
			Msg("webhook delivery failed")
		return err
	}

	metricDeliveries.WithLabelValues("ok").Inc()
	s.log.Info().
		Int("org_id", ev.OrgID).
		Int("status", status).
		Str("delivery_id", ev.DeliveryID).
		Msg("webhook delivered")
	return nil
}
