package twilio_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/twilio"
)

// This fails if recording Twilio boundary outcomes does not produce the
// documented bounded metric families and observable values.
func TestMetrics_RecordOutcomes(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := twilio.NewMetrics(registry)
	require.NoError(t, err)

	metrics.RecordSubmission(twilio.SubmissionAccepted)
	metrics.RecordSubmission(twilio.SubmissionTransientError)
	metrics.RecordCallback(twilio.CallbackStatus, twilio.CallbackAccepted)
	metrics.RecordCallback(twilio.CallbackInbound, twilio.CallbackInvalidSignature)
	metrics.ObserveRequestDuration(40 * time.Millisecond)
	metrics.ObserveRequestDuration(60 * time.Millisecond)

	var gatherer prometheus.Gatherer = registry
	families, err := gatherer.Gather()
	require.NoError(t, err)

	names := make([]string, 0, len(families))
	submissions := map[string]float64{}
	callbacks := map[string]float64{}
	for _, family := range families {
		names = append(names, family.GetName())
		switch family.GetName() {
		case "trakrf_twilio_submissions_total":
			for _, metric := range family.GetMetric() {
				require.Len(t, metric.GetLabel(), 1)
				submissions[metric.GetLabel()[0].GetName()+"="+metric.GetLabel()[0].GetValue()] = metric.GetCounter().GetValue()
			}
		case "trakrf_twilio_callbacks_total":
			for _, metric := range family.GetMetric() {
				require.Len(t, metric.GetLabel(), 2)
				labels := map[string]string{}
				for _, label := range metric.GetLabel() {
					labels[label.GetName()] = label.GetValue()
				}
				callbacks[labels["type"]+"/"+labels["result"]] = metric.GetCounter().GetValue()
			}
		case "trakrf_twilio_request_duration_seconds":
			require.Len(t, family.GetMetric(), 1)
			metric := family.GetMetric()[0]
			require.Empty(t, metric.GetLabel())
			require.Equal(t, uint64(2), metric.GetHistogram().GetSampleCount())
			require.InEpsilon(t, 0.1, metric.GetHistogram().GetSampleSum(), 0.00001)
		}
	}
	sort.Strings(names)
	require.Equal(t, []string{
		"trakrf_twilio_callbacks_total",
		"trakrf_twilio_request_duration_seconds",
		"trakrf_twilio_submissions_total",
	}, names)
	require.Equal(t, map[string]float64{"result=accepted": 1, "result=transient": 1}, submissions)
	require.Equal(t, map[string]float64{"inbound/invalid_signature": 1, "status/accepted": 1}, callbacks)
}

// This fails if arbitrary caller input is emitted as a Prometheus label value,
// which would leak sensitive data and allow unbounded time-series cardinality.
func TestMetrics_NormalizesUnknownInputsToFixedLabels(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := twilio.NewMetrics(registry)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		metrics.RecordSubmission(twilio.SubmissionResult(fmt.Sprintf("org-42 delivery-9 body-%d", i)))
		metrics.RecordCallback(
			twilio.CallbackType(fmt.Sprintf("+1555555012%d", i)),
			twilio.CallbackResult(fmt.Sprintf("SM%d error credentials-secret", i)),
		)
	}

	var gatherer prometheus.Gatherer = registry
	families, err := gatherer.Gather()
	require.NoError(t, err)
	require.Len(t, families, 3)

	for _, family := range families {
		require.Len(t, family.GetMetric(), 1)
		metric := family.GetMetric()[0]
		switch family.GetName() {
		case "trakrf_twilio_submissions_total":
			require.Equal(t, float64(3), metric.GetCounter().GetValue())
			require.Len(t, metric.GetLabel(), 1)
			require.Equal(t, "result", metric.GetLabel()[0].GetName())
			require.Equal(t, "unknown", metric.GetLabel()[0].GetValue())
		case "trakrf_twilio_callbacks_total":
			require.Equal(t, float64(3), metric.GetCounter().GetValue())
			require.Len(t, metric.GetLabel(), 2)
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			require.Equal(t, map[string]string{"type": "unknown", "result": "unknown"}, labels)
		case "trakrf_twilio_request_duration_seconds":
			require.Empty(t, metric.GetLabel())
			require.Equal(t, uint64(0), metric.GetHistogram().GetSampleCount())
		default:
			t.Fatalf("unexpected metric family %q", family.GetName())
		}
	}
}

// This fails if a duplicate collector registration panics or is silently
// accepted instead of returning the Prometheus registration error to callers.
func TestMetrics_DuplicateRegistrationReturnsError(t *testing.T) {
	registry := prometheus.NewRegistry()
	_, err := twilio.NewMetrics(registry)
	require.NoError(t, err)

	duplicate, err := twilio.NewMetrics(registry)

	require.Nil(t, duplicate)
	require.Error(t, err)
}
