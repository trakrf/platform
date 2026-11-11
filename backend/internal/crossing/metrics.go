package crossing

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// metricCrossings counts outcomes by result, each counted once: recorded is
	// a crossing from tracked state, cold_start one from a seed, deduped a repeat
	// or a replay that wrote no row, and tie a run of tied reads.
	metricCrossings = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "crossing_total",
		Help: "Gate crossing outcomes by result (recorded, cold_start, deduped, tie); results do not overlap.",
	}, []string{"result"})

	metricReadsIgnored = promauto.NewCounter(prometheus.CounterOpts{
		Name: "crossing_reads_ignored_total",
		Help: "Reads skipped by crossing detection for lacking a usable RSSI.",
	})
)
