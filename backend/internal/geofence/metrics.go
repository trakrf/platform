package geofence

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Counters live on the default registry, which serve's /metrics handler exposes.
// They make every geofence decision observable alongside the ingest counters.
var (
	metricEvaluated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "geofence_evaluated_total",
		Help: "Membership-passing reads evaluated by the geofence engine.",
	})

	metricFired = promauto.NewCounter(prometheus.CounterOpts{
		Name: "geofence_alarms_fired_total",
		Help: "Geofence boundary alarms fired (post-dedup).",
	})

	metricSuppressed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "geofence_alarms_suppressed_total",
		Help: "Reads that did not fire, by reason.",
	}, []string{"reason"}) // no_rssi, no_location, rssi_below_threshold, latched, startup_grace

	metricFireErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "geofence_fire_errors_total",
		Help: "Errors driving output devices or looking them up (best-effort; do not block ingestion).",
	})

	metricEventWriteErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "geofence_event_write_errors_total",
		Help: "Errors writing alarm_events rows (best-effort; do not block ingestion).",
	})
)
