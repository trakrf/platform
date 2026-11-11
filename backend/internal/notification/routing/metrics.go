package routing

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
)

// Metrics counts planning outcomes by source. Owns no global registration;
// callers supply the registerer that owns its lifecycle (mirrors
// backend/internal/notification/outbox/metrics.go). It counts only and never
// records who was notified.
type Metrics struct {
	outcomes *prometheus.CounterVec
}

func NewMetrics(registerer prometheus.Registerer) (*Metrics, error) {
	if registerer == nil {
		return nil, errors.New("routing metrics registerer is required")
	}

	metrics := &Metrics{
		outcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trakrf_notification_delivery_planning_outcomes_total",
			Help: "Notification delivery planning outcomes by source and bounded outcome.",
		}, []string{"source", "outcome"}),
	}

	if err := registerer.Register(metrics.outcomes); err != nil {
		return nil, err
	}

	return metrics, nil
}

func (m *Metrics) RecordOutcome(source notificationevent.Source, outcome Outcome) {
	m.outcomes.WithLabelValues(string(source), string(outcome)).Inc()
}
