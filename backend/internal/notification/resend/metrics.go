package resend

import (
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/notification/email"
)

// Metrics owns bounded boundary telemetry. No recipient, payload, identifier,
// signature, secret or provider error text is recorded. Registration is local
// to the supplied registry; duplicates return an error rather than panicking.
type Metrics struct {
	submissions *prometheus.CounterVec
	callbacks   *prometheus.CounterVec
	duration    prometheus.Histogram
	log         *zerolog.Logger
}

func NewMetrics(registry prometheus.Registerer) (*Metrics, error) {
	if registry == nil {
		return nil, errors.New("email metrics registerer is required")
	}
	m := &Metrics{
		submissions: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "trakrf_resend_submissions_total", Help: "Notification email submission outcomes; acceptance is not delivery."}, []string{"result", "outcome_unknown"}),
		callbacks:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "trakrf_resend_callbacks_total", Help: "Verified email callback handoff outcomes, including duplicate receipts."}, []string{"result"}),
		duration:    prometheus.NewHistogram(prometheus.HistogramOpts{Name: "trakrf_resend_request_duration_seconds", Help: "Notification email submission duration.", Buckets: prometheus.DefBuckets}),
		log:         logger.Get(),
	}
	collectors := []prometheus.Collector{m.submissions, m.callbacks, m.duration}
	for i, collector := range collectors {
		if err := registry.Register(collector); err != nil {
			for _, registered := range collectors[:i] {
				registry.Unregister(registered)
			}
			return nil, err
		}
	}
	return m, nil
}

func (m *Metrics) RecordSubmission(err error, duration time.Duration) {
	if m == nil {
		return
	}
	result, unknown := "accepted", false
	if err != nil {
		result, unknown = "unknown", true
		var failure *email.ProviderError
		if errors.As(err, &failure) && failure != nil {
			unknown = failure.OutcomeUnknown
			switch failure.Kind {
			case email.ErrorInvalid, email.ErrorDisabled, email.ErrorPermanent, email.ErrorTransient, email.ErrorCanceled, email.ErrorTimeout:
				result = string(failure.Kind)
			}
		}
	}
	ambiguity := "false"
	if unknown {
		ambiguity = "true"
	}
	m.submissions.WithLabelValues(result, ambiguity).Inc()
	if duration >= 0 {
		m.duration.Observe(duration.Seconds())
	}
	m.log.Debug().Str("result", result).Bool("outcome_unknown", unknown).Msg("Notification email submission")
}

// RecordCallback counts acknowledgments, not unique deliveries. Unsupported
// authenticated events and durable handoffs have separate outcomes.
func (m *Metrics) RecordCallback(result string) {
	if m == nil {
		return
	}
	switch result {
	case "persisted", "ignored", "invalid_signature", "malformed", "too_large", "method_not_allowed", "consumer_failure":
	default:
		result = "unknown"
	}
	m.callbacks.WithLabelValues(result).Inc()
	m.log.Debug().Str("result", result).Msg("Notification email callback")
}
