package resend

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/notification/email"
)

func TestMetricsBoundedAndRedacted(t *testing.T) {
	var logs bytes.Buffer
	original := *logger.Get()
	logger.SetForTest(zerolog.New(&logs).Level(zerolog.DebugLevel))
	t.Cleanup(func() { logger.SetForTest(original) })
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	require.NoError(t, err)
	for _, failure := range []error{nil, errors.New("private raw failure"), &email.ProviderError{Kind: "private kind", OutcomeUnknown: true}, &email.ProviderError{Kind: email.ErrorTimeout, OutcomeUnknown: true}} {
		metrics.RecordSubmission(failure, time.Millisecond)
	}
	for _, result := range []string{"persisted", "ignored", "invalid_signature", "malformed", "too_large", "method_not_allowed", "consumer_failure", "private callback"} {
		metrics.RecordCallback(result)
	}
	require.Equal(t, float64(1), counterValue(t, metrics.submissions.WithLabelValues("accepted", "false")))
	require.Equal(t, float64(2), counterValue(t, metrics.submissions.WithLabelValues("unknown", "true")))
	require.Equal(t, float64(1), counterValue(t, metrics.submissions.WithLabelValues("timeout", "true")))
	require.Equal(t, float64(1), counterValue(t, metrics.callbacks.WithLabelValues("unknown")))
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		require.NotContains(t, family.String(), "private")
	}
	require.NotContains(t, logs.String(), "private")
	require.Contains(t, logs.String(), "outcome_unknown")
}

func TestMetricsRegistrationRollback(t *testing.T) {
	registry := prometheus.NewRegistry()
	existing := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "trakrf_resend_callbacks_total", Help: "Verified email callback handoff outcomes, including duplicate receipts."}, []string{"result"})
	require.NoError(t, registry.Register(existing))
	_, err := NewMetrics(registry)
	require.Error(t, err)
	require.True(t, registry.Unregister(existing))
	_, err = NewMetrics(registry)
	require.NoError(t, err, "failed registration must release earlier collectors")
}

func counterValue(t *testing.T, counter prometheus.Counter) float64 {
	t.Helper()
	metric := &dto.Metric{}
	require.NoError(t, counter.Write(metric))
	return metric.GetCounter().GetValue()
}
