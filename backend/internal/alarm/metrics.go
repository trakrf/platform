package alarm

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const deviceCallMetricName = "alarm_device_call_duration_seconds"

// kind is the call path, one per transport/frame pair the Dispatcher drives.
// It is finer than the device's transport column because both Shelly-over-MQTT
// and the reader GPO ride mqtt but have very different latency profiles.
const (
	kindShellyHTTP = "shelly_http"
	kindShellyMQTT = "shelly_mqtt"
	kindCSLGPO     = "csl_gpo"
)

const (
	outcomeSuccess = "success"
	outcomeError   = "error"
	outcomeTimeout = "timeout"
)

// ErrPublishTimeout is wrapped by MQTT publishes that the broker did not
// acknowledge in time, so callers (and the call histogram) can tell a timeout
// from other failures. It reports Timeout() like a net.Error.
var ErrPublishTimeout error = publishTimeoutError{}

type publishTimeoutError struct{}

func (publishTimeoutError) Error() string { return "publish timed out" }
func (publishTimeoutError) Timeout() bool { return true }

// DeviceCallMetrics times each output-device call the Dispatcher makes. A nil
// *DeviceCallMetrics records nothing.
type DeviceCallMetrics struct {
	duration *prometheus.HistogramVec
}

// NewDeviceCallMetrics registers the call histogram on registerer, reusing the
// existing collector when one is already registered.
func NewDeviceCallMetrics(registerer prometheus.Registerer) *DeviceCallMetrics {
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    deviceCallMetricName,
		Help:    "Duration of output-device (Shelly / reader GPO) calls, by call path and outcome.",
		Buckets: prometheus.DefBuckets, // 5ms..10s spans the 3s Shelly and 5s publish timeouts
	}, []string{"kind", "outcome"})
	if err := registerer.Register(duration); err != nil {
		var already prometheus.AlreadyRegisteredError
		if !errors.As(err, &already) {
			panic(err)
		}
		duration = already.ExistingCollector.(*prometheus.HistogramVec)
	}
	return &DeviceCallMetrics{duration: duration}
}

// time runs call and records its duration under kind and the call's outcome.
func (m *DeviceCallMetrics) time(kind string, call func() error) error {
	if m == nil {
		return call()
	}
	start := time.Now()
	err := call()
	m.duration.WithLabelValues(kind, callOutcome(err)).Observe(time.Since(start).Seconds())
	return err
}

func callOutcome(err error) string {
	if err == nil {
		return outcomeSuccess
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return outcomeTimeout
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return outcomeTimeout
	}
	return outcomeError
}
