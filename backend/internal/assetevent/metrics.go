package assetevent

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Counters live on the default registry, which serve's /metrics handler
// exposes. They make every detection and delivery decision observable
// alongside the ingest and geofence counters.
var (
	metricEvaluated = promauto.NewCounter(prometheus.CounterOpts{
		Name: "asset_events_evaluated_total",
		Help: "Scan observations considered for an asset.moved delta.",
	})

	metricEmitted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "asset_events_emitted_total",
		Help: "asset.moved events detected and enqueued for delivery.",
	})

	metricSuppressed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "asset_events_suppressed_total",
		Help: "Observations that produced no event, by reason.",
	}, []string{"reason"}) // not_stored, no_location, no_change, unresolved_names

	metricDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "asset_events_dropped_total",
		Help: "Detected events that were never delivered, by reason. This is the at-most-once loss counter.",
	}, []string{"reason"}) // queue_full, retries_exhausted, shutdown

	metricLookupErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "asset_events_lookup_errors_total",
		Help: "Previous-location or name lookups that failed (best-effort; never blocks ingestion).",
	})

	metricLookupSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "asset_events_lookup_seconds",
		Help:    "Duration of the batched previous-location lookup (CAGG + base tail).",
		Buckets: prometheus.DefBuckets,
	})

	metricDeliveryAttempts = promauto.NewCounter(prometheus.CounterOpts{
		Name: "asset_events_delivery_attempts_total",
		Help: "Sink delivery attempts, including retries.",
	})

	metricDeliveryErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "asset_events_delivery_errors_total",
		Help: "Sink delivery attempts that returned an error.",
	})
)
