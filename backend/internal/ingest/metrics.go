package ingest

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Counters live on the default registry, which serve's /metrics handler exposes.
// They replace the old trigger's silent EXCEPTION WHEN OTHERS with observable
// outcomes for every message and every dropped read.
var (
	metricMessages = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ingest_messages_total",
		Help: "MQTT messages received by the in-backend subscriber, by result.",
	}, []string{"result"}) // received, unregistered_topic, unsupported_device, parse_error, panic, audit_error, resolve_error, derive_error

	metricReadsParsed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ingest_reads_parsed_total",
		Help: "Tag reads parsed from MQTT payloads.",
	})

	metricAssetScansInserted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ingest_asset_scans_inserted_total",
		Help: "asset_scans rows inserted by the subscriber.",
	})

	metricReadsDropped = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "ingest_reads_dropped_total",
		Help: "Parsed reads dropped during derivation, by reason.",
	}, []string{"reason"}) // no_scan_point, no_asset, conflict
)
