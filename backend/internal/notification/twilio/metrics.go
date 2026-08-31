package twilio

import (
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricSubmissionResultUnknown = "unknown"
	metricCallbackTypeUnknown     = "unknown"
	metricCallbackResultUnknown   = "unknown"
)

// SubmissionResult is the bounded outcome of a Twilio submission attempt.
type SubmissionResult string

const (
	SubmissionAccepted       SubmissionResult = "accepted"
	SubmissionTransientError SubmissionResult = "transient"
	SubmissionPermanentError SubmissionResult = "permanent"
	SubmissionRejected       SubmissionResult = "rejected"
)

// CallbackType identifies the bounded Twilio callback endpoint that produced
// an outcome.
type CallbackType string

const (
	CallbackStatus  CallbackType = "status"
	CallbackInbound CallbackType = "inbound"
)

// CallbackResult is the bounded outcome of a Twilio callback attempt.
type CallbackResult string

const (
	CallbackAccepted         CallbackResult = "accepted"
	CallbackInvalidSignature CallbackResult = "invalid_signature"
	CallbackMalformed        CallbackResult = "malformed"
	CallbackConsumerFailure  CallbackResult = "consumer_failure"
)

// Metrics records bounded Twilio boundary outcomes. It owns no global
// Prometheus registration; callers supply the registerer that owns its
// lifecycle.
type Metrics struct {
	submissions     *prometheus.CounterVec
	callbacks       *prometheus.CounterVec
	requestDuration prometheus.Histogram
}

// NewMetrics creates and registers the Twilio collectors with registerer.
// Registration errors, including duplicate metric names, are returned to the
// caller without using Prometheus's panic-on-registration helpers.
func NewMetrics(registerer prometheus.Registerer) (*Metrics, error) {
	if registerer == nil {
		return nil, errors.New("Twilio metrics registerer is required")
	}

	metrics := &Metrics{
		submissions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trakrf_twilio_submissions_total",
			Help: "Twilio SMS submission outcomes by bounded result.",
		}, []string{"result"}),
		callbacks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "trakrf_twilio_callbacks_total",
			Help: "Twilio callback outcomes by bounded type and result.",
		}, []string{"type", "result"}),
		requestDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "trakrf_twilio_request_duration_seconds",
			Help:    "Duration of Twilio boundary requests.",
			Buckets: prometheus.DefBuckets,
		}),
	}

	if err := registerer.Register(metrics.submissions); err != nil {
		return nil, err
	}
	if err := registerer.Register(metrics.callbacks); err != nil {
		registerer.Unregister(metrics.submissions)
		return nil, err
	}
	if err := registerer.Register(metrics.requestDuration); err != nil {
		registerer.Unregister(metrics.callbacks)
		registerer.Unregister(metrics.submissions)
		return nil, err
	}

	return metrics, nil
}

// RecordSubmission records one submission outcome after normalizing it to a
// finite, non-sensitive result label.
func (metrics *Metrics) RecordSubmission(result SubmissionResult) {
	metrics.submissions.WithLabelValues(normalizeSubmissionResult(result)).Inc()
}

// RecordCallback records one callback outcome after normalizing its type and
// result to finite, non-sensitive label values.
func (metrics *Metrics) RecordCallback(callbackType CallbackType, result CallbackResult) {
	metrics.callbacks.WithLabelValues(normalizeCallbackType(callbackType), normalizeCallbackResult(result)).Inc()
}

// ObserveRequestDuration records the duration of one Twilio boundary request.
func (metrics *Metrics) ObserveRequestDuration(duration time.Duration) {
	metrics.requestDuration.Observe(duration.Seconds())
}

func normalizeSubmissionResult(result SubmissionResult) string {
	switch result {
	case SubmissionAccepted, SubmissionTransientError, SubmissionPermanentError, SubmissionRejected:
		return string(result)
	default:
		return metricSubmissionResultUnknown
	}
}

func normalizeCallbackType(callbackType CallbackType) string {
	switch callbackType {
	case CallbackStatus, CallbackInbound:
		return string(callbackType)
	default:
		return metricCallbackTypeUnknown
	}
}

func normalizeCallbackResult(result CallbackResult) string {
	switch result {
	case CallbackAccepted, CallbackInvalidSignature, CallbackMalformed, CallbackConsumerFailure:
		return string(result)
	default:
		return metricCallbackResultUnknown
	}
}
