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

func TestMetrics_RecordEnqueue_CountsDuplicates(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	metrics.RecordEnqueue(outbox.EnqueueDuplicate)
	metrics.RecordEnqueue(outbox.EnqueueDuplicate)

	require.Equal(t, float64(2), enqueueCount(t, registry, "duplicate"))
}

// enqueueCount reads trakrf_outbox_enqueues_total for one result label;
// a label never recorded reads as 0.
func enqueueCount(t *testing.T, registry *prometheus.Registry, result string) float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "trakrf_outbox_enqueues_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "result" && label.GetValue() == result {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}
