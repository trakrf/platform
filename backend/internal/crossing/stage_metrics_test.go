package crossing

import (
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/storage"
)

type counts struct{ recorded, cold, deduped, tie, ignored float64 }

func read(t *testing.T) counts {
	t.Helper()
	val := func(w interface{ Write(*dto.Metric) error }) float64 {
		var m dto.Metric
		require.NoError(t, w.Write(&m))
		return m.GetCounter().GetValue()
	}
	return counts{
		recorded: val(metricCrossings.WithLabelValues("recorded")),
		cold:     val(metricCrossings.WithLabelValues("cold_start")),
		deduped:  val(metricCrossings.WithLabelValues("deduped")),
		tie:      val(metricCrossings.WithLabelValues("tie")),
		ignored:  val(metricReadsIgnored),
	}
}

func (c counts) since(before counts) counts {
	return counts{c.recorded - before.recorded, c.cold - before.cold, c.deduped - before.deduped, c.tie - before.tie, c.ignored - before.ignored}
}

// commitMetrics runs one message and returns the metric change it caused before
// and after the scan committed.
func commitMetrics(t *testing.T, s *Stage, in storage.DetectInput, rowsKept bool) (beforeCommit, afterCommit counts) {
	t.Helper()
	start := read(t)
	commit, err := detect(t, s, in)
	require.NoError(t, err)
	beforeCommit = read(t).since(start)
	commit(rowsKept)
	return beforeCommit, read(t).since(start)
}

func TestStage_MetricsAfterCommit(t *testing.T) {
	s, _, _ := seeded()
	// Asset 42: cold start crossing A to B. Asset 43: new at A, tied by a B read.
	// Asset 44: no usable RSSI.
	before, after := commitMetrics(t, s, input(recvAt,
		rd(testAsset, 202, locB, -50, time.Time{}),
		rd(43, 201, locA, -45, time.Time{}),
		rd(43, 202, locB, -44, time.Time{}),
		rd(44, 201, locA, 0, time.Time{}),
	), true)

	assert.Equal(t, counts{}, before, "nothing counts before the commit")
	assert.Equal(t, counts{cold: 1, tie: 1, ignored: 1}, after, "a cold crossing counts as cold_start only")
}

func TestStage_DiscardedStageCountsNoCrossings(t *testing.T) {
	s, _, _ := seeded()
	_, after := commitMetrics(t, s, input(recvAt,
		rd(testAsset, 202, locB, -50, time.Time{}), rd(44, 201, locA, 0, time.Time{})), false)
	assert.Equal(t, counts{ignored: 1}, after)
}

func TestStage_ReplayedCrossingCountsAsDedupedOnly(t *testing.T) {
	t.Run("warm", func(t *testing.T) {
		s, _, rec := warm(t)
		rec.replay = true
		_, after := commitMetrics(t, s, input(recvAt, rd(testAsset, 202, locB, -50, time.Time{})), true)
		assert.Equal(t, counts{deduped: 1}, after)
	})
	t.Run("cold", func(t *testing.T) {
		s, _, rec := seeded()
		rec.replay = true
		_, after := commitMetrics(t, s, input(recvAt, rd(testAsset, 202, locB, -50, time.Time{})), true)
		assert.Equal(t, counts{deduped: 1}, after)
	})
}

// A flapping asset: the tracker's own dedup reaches the metric, and writes no row.
func TestStage_TrackerDedupCounts(t *testing.T) {
	s, _, rec := warm(t)
	var total counts
	for i, l := range []int{locB, locA, locB} {
		_, after := commitMetrics(t, s, input(recvAt.Add(time.Duration(5*(i+1))*time.Second),
			rd(testAsset, 200+l, l, -50, time.Time{})), true)
		total = counts{total.recorded + after.recorded, 0, total.deduped + after.deduped, 0, 0}
	}
	assert.Equal(t, counts{recorded: 2, deduped: 1}, total)
	assert.Len(t, rec.got, 2)
}
