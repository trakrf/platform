package webhook

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// metricDeliveries counts what happened to each event handed to the sink.
	//
	// The skip reasons are separate labels on purpose: "skipped, unentitled" and
	// "dropped, queue full" are entirely different operational signals — one is
	// billing, the other is capacity — and collapsing them would hide both.
	metricDeliveries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook_deliveries_total",
		Help: "Webhook delivery outcomes by result.",
	}, []string{"result"}) // ok, failed, skipped_no_webhook, skipped_disabled, skipped_unentitled

	metricLookupErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "webhook_lookup_errors_total",
		Help: "Failures resolving an org's webhook row for delivery.",
	})

	metricDeliverySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "webhook_delivery_seconds",
		Help:    "Duration of an outbound webhook POST.",
		Buckets: prometheus.DefBuckets,
	})
)
