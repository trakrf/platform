package outbox_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/notification/outbox"
)

func TestMetrics_RecordEnqueue_IncrementsBoundedLabel(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	metrics.RecordEnqueue(outbox.EnqueueSkippedUnentitled)

	families, err := registry.Gather()
	require.NoError(t, err)
	found := false
	for _, family := range families {
		if family.GetName() != "trakrf_outbox_enqueues_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "result" && label.GetValue() == "skipped_unentitled" {
					found = true
					require.Equal(t, float64(1), metric.GetCounter().GetValue())
				}
			}
		}
	}
	require.True(t, found, "expected a skipped_unentitled enqueue metric")
}
