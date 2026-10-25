package routing_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/notification/routing"
)

func TestNewMetrics_RequiresRegisterer(t *testing.T) {
	metrics, err := routing.NewMetrics(nil)
	require.Error(t, err)
	require.Nil(t, metrics)
}

func TestRecordOutcome_CountsBySourceAndOutcome(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := routing.NewMetrics(registry)
	require.NoError(t, err)

	metrics.RecordOutcome(notificationevent.SourceAlarm, routing.OutcomeEnqueued)
	metrics.RecordOutcome(notificationevent.SourceAlarm, routing.OutcomeEnqueued)
	metrics.RecordOutcome(notificationevent.SourceLocationChange, routing.OutcomeEnqueued)

	families, err := registry.Gather()
	require.NoError(t, err)
	names := make([]string, 0, len(families))
	for _, family := range families {
		names = append(names, family.GetName())
	}
	require.Contains(t, names, "trakrf_notification_delivery_planning_outcomes_total")

	counter := func(source, outcome string) float64 {
		return seriesValue(t, registry, source, outcome)
	}
	require.Equal(t, float64(2), counter("alarm", "enqueued"))
	require.Equal(t, float64(1), counter("location_change", "enqueued"))
}

// seriesValue reads one labelled series of the planning counter.
// testutil is avoided: it needs go.mod changes this task must not make.
func seriesValue(t *testing.T, registry *prometheus.Registry, source, outcome string) float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "trakrf_notification_delivery_planning_outcomes_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["source"] == source && labels["outcome"] == outcome {
				return metric.GetCounter().GetValue()
			}
		}
	}
	t.Fatalf("no series for source=%s outcome=%s", source, outcome)
	return 0
}
