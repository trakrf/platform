package outbox

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

// EnqueueResult is the bounded outcome of an Enqueue call.
type EnqueueResult string

const (
	EnqueueOK                EnqueueResult = "ok"
	EnqueueSkippedUnentitled EnqueueResult = "skipped_unentitled"
	// EnqueueDuplicate: the delivery ID was already in the outbox, so no
	// second row or job was created.
	EnqueueDuplicate EnqueueResult = "duplicate"
)

// DeliveryOutcome is the bounded outcome of one worker attempt.
type DeliveryOutcome string

const (
	DeliveryDelivered        DeliveryOutcome = "delivered"
	DeliveryRetryableFailure DeliveryOutcome = "retryable_failure"
	DeliveryPermanentFailure DeliveryOutcome = "permanent_failure"
)

// Metrics records bounded outbox outcomes. Owns no global registration;
// callers supply the registerer that owns its lifecycle (mirrors
// backend/internal/notification/twilio/metrics.go).
type Metrics struct {
	enqueues         *prometheus.CounterVec
	deliveries       *prometheus.CounterVec
	oldestPendingAge prometheus.Gauge
}

func NewMetrics(registerer prometheus.Registerer) (*Metrics, error) {
	if registerer == nil {
		return nil, errors.New("outbox metrics registerer is required")
	}

	metrics := &Metrics{
		enqueues: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trakrf_outbox_enqueues_total",
			Help: "Notification outbox enqueue outcomes by bounded result.",
		}, []string{"result"}),
		deliveries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trakrf_outbox_deliveries_total",
			Help: "Notification outbox delivery attempt outcomes by bounded result.",
		}, []string{"result"}),
		oldestPendingAge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "trakrf_outbox_oldest_pending_seconds",
			Help: "Age in seconds of the oldest pending notification delivery.",
		}),
	}

	if err := registerer.Register(metrics.enqueues); err != nil {
		return nil, err
	}
	if err := registerer.Register(metrics.deliveries); err != nil {
		registerer.Unregister(metrics.enqueues)
		return nil, err
	}
	if err := registerer.Register(metrics.oldestPendingAge); err != nil {
		registerer.Unregister(metrics.deliveries)
		registerer.Unregister(metrics.enqueues)
		return nil, err
	}

	return metrics, nil
}

func (m *Metrics) RecordEnqueue(result EnqueueResult) {
	m.enqueues.WithLabelValues(string(result)).Inc()
}

func (m *Metrics) RecordDelivery(outcome DeliveryOutcome) {
	m.deliveries.WithLabelValues(string(outcome)).Inc()
}

func (m *Metrics) SetOldestPendingAgeSeconds(seconds float64) {
	m.oldestPendingAge.Set(seconds)
}
