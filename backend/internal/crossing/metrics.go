package crossing

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// metricCrossings counts outcomes by result, each counted once: recorded is
	// a crossing that wrote a row, deduped a repeat or a replay that wrote none,
	// and tie a run of tied reads.
	metricCrossings = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "crossing_total",
		Help: "Gate crossing outcomes by result (recorded, deduped, tie); results do not overlap.",
	}, []string{"result"})

	metricReadsIgnored = promauto.NewCounter(prometheus.CounterOpts{
		Name: "crossing_reads_ignored_total",
		Help: "Reads skipped by crossing detection for lacking a usable RSSI.",
	})
)
