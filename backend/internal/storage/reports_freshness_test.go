package storage

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TRA-1117 structural guards on the current-locations SQL. These are DB-free on
// purpose: Playwright never runs in CI and the integration suite is behind a
// build tag, so this file is the check that actually fires on every PR. The
// integration tests next door prove the behaviour; these prove the shape that
// behaviour depends on cannot be quietly refactored away.

// renderedCurrentLocationsQueries returns both queries the report issues, with
// whitespace normalised so assertions do not depend on indentation.
func renderedCurrentLocationsQueries() (list, count string) {
	list = normalizeSQL(buildCurrentLocationsQuery("ls.last_seen DESC", "p.last_seen DESC"))
	count = normalizeSQL(countCurrentLocationsQuery())
	return
}

func normalizeSQL(q string) string {
	return strings.Join(strings.Fields(q), " ")
}

// The whole point of the ticket: both queries must read raw asset_scans, not the
// CAGG alone. A regression here is silent — the report still works, it just goes
// back to being two minutes behind.
func TestCurrentLocationsQueries_ReadTheFreshTail(t *testing.T) {
	list, count := renderedCurrentLocationsQueries()

	for name, q := range map[string]string{"list": list, "count": count} {
		assert.Contains(t, q, "FROM trakrf.asset_scans s",
			"%s query must read the raw tail or the report lags materialization", name)
		assert.Contains(t, q, "FROM trakrf.asset_scan_latest",
			"%s query must still take history from the CAGG", name)
		assert.Contains(t, q, "fresh_tail AS MATERIALIZED (",
			"%s query: asset_scans cannot be pruned at plan time, so the tail is planned once", name)
	}
}

// The dwell probes are the third consumer, and the easiest to forget: miss them
// and locations are fresh while dwell is stale or null on exactly the row the
// user just created.
func TestCurrentLocationsQuery_DwellSeesTheFreshTail(t *testing.T) {
	list, _ := renderedCurrentLocationsQueries()

	assert.Contains(t, list, "FROM fresh_tail t WHERE t.asset_id = p.asset_id AND t.location_id IS DISTINCT FROM p.scan_location_id",
		"run detection must consider a move recorded only in the tail")
	assert.Contains(t, list, "FROM fresh_tail t WHERE t.asset_id = p.asset_id AND t.bucket > w.run_after",
		"the run-start probe must fall through to the tail")
}

// Both dwell probes ask one source at a time and COALESCE, rather than ordering
// over a union. A union cannot use the per-chunk index for max()/LIMIT 1; on
// preview that was 41ms per row against ~1ms. The COALESCE order encodes the
// disjointness — tail first for the newest, CAGG first for the oldest — so
// reordering the arms is a correctness bug, not a style change.
func TestCurrentLocationsQuery_DwellProbesOneSourceAtATime(t *testing.T) {
	list, _ := renderedCurrentLocationsQueries()

	// Newest-first: tail arm precedes CAGG arm.
	runAfter := strings.Index(list, "AS run_after")
	require.Positive(t, runAfter, "the run-detection LATERAL must exist")
	head := list[:runAfter]
	assert.Less(t, strings.LastIndex(head, "SELECT max(t.bucket) FROM fresh_tail t"),
		strings.LastIndex(head, "SELECT max(c.bucket) FROM trakrf.asset_scan_latest c"),
		"tail holds the newest buckets, so it must be the first COALESCE arm")

	// Oldest-first: CAGG arm precedes tail arm.
	dwell := strings.Index(list, "AS dwell_started_at")
	require.Positive(t, dwell, "the run-start LATERAL must exist")
	tailProbe := strings.Index(list[runAfter:dwell], "SELECT t.last_seen FROM fresh_tail t")
	caggProbe := strings.Index(list[runAfter:dwell], "SELECT c.last_seen FROM trakrf.asset_scan_latest c")
	require.Positive(t, tailProbe)
	require.Positive(t, caggProbe)
	assert.Less(t, caggProbe, tailProbe,
		"the CAGG holds the oldest buckets, so it must be the first COALESCE arm")
}

// TRA-1021: a DISTINCT ON over asset_scans tripped a TimescaleDB SkipScan bug
// that XX000-crashed preview, and TRA-1022 moved to the CAGG to escape it. Now
// that the raw hypertable is back in this query, the ban has to be enforced
// rather than remembered.
func TestCurrentLocationsQueries_NeverUseDistinctOn(t *testing.T) {
	list, count := renderedCurrentLocationsQueries()

	assert.NotContains(t, strings.ToUpper(list), "DISTINCT ON")
	assert.NotContains(t, strings.ToUpper(count), "DISTINCT ON")
}

// The cut must be a bucket boundary, or the split between the two sources is
// approximate rather than exact and a bucket can be double-counted into run
// detection or dropped from it.
func TestFreshTailCut_IsBucketAligned(t *testing.T) {
	cut := normalizeSQL(freshTailCut)

	assert.Contains(t, cut, "time_bucket(INTERVAL '1 minute'",
		"the cut must land on a bucket boundary for the split to be exact")
	assert.Contains(t, cut, freshScanTailWindow)
}

// Where the cut applies is the subtle part, and it differs by consumer.
//
// Run detection MUST bound the CAGG below the cut: a bucket materialized while
// still filling holds a stale last(location_id) that reads as a visit elsewhere
// and truncates the run.
//
// The latest_scans roll-up must NOT, and this is the one measured on preview
// (500k buckets): on an un-correlated whole-org aggregate the bound loses the
// index and drops onto a parallel seq scan of every chunk. It is safe to omit
// precisely because that roll-up only wants the newest observation, and the CAGG
// can never hold a later last_seen than the raw rows it derives from.
func TestCurrentLocationsQueries_CutBoundsRunDetectionOnly(t *testing.T) {
	list, count := renderedCurrentLocationsQueries()
	cut := normalizeSQL(freshTailCut)
	rollup := normalizeSQL(latestScansCTE())

	assert.NotContains(t, rollup, cut,
		"bounding the whole-org roll-up by the cut costs the index; see freshTailCut")

	for name, q := range map[string]string{"list": list, "count": count} {
		assert.Contains(t, q, "s.timestamp >= "+cut, "%s query: the tail starts at the cut", name)
	}

	// Both per-asset dwell probes bound the CAGG; the roll-up does not.
	assert.Equal(t, 2, strings.Count(list, "c.bucket < "+cut),
		"both dwell probes must exclude unsettled CAGG buckets")
	assert.NotContains(t, count, "c.bucket < "+cut,
		"the count resolves no runs, so it has no dwell probes to bound")
}

// The tail must be wider than the worst-case materialization lag — end_offset
// (1 min) + schedule_interval (30s) + a slipped cycle. Narrowing it below that
// silently reopens the invisibility window with no other symptom.
func TestFreshScanTailWindow_ClearsTheMaterializationLag(t *testing.T) {
	assert.GreaterOrEqual(t, freshScanTailMinutes, 3,
		"the tail must clear end_offset + schedule_interval + a slipped cycle; see 000028_asset_scan_latest_policy")
	assert.Equal(t, "5 minutes", freshScanTailWindow)
}

// The chunk-exclusion bound must stay LOOSER than the cut, by at least the
// bucket width that truncation can subtract. Tighten it and it starts rejecting
// rows the cut would have kept — silently, since it sits alongside the cut and
// nothing else would notice a row going missing from the tail.
func TestFreshTailChunkBound_IsLooserThanTheCut(t *testing.T) {
	assert.Equal(t, "6 minutes", freshTailChunkWindow)
	assert.Greater(t, freshScanTailMinutes+caggBucketMinutes, freshScanTailMinutes,
		"the chunk bound must reach back at least one bucket further than the cut")

	// And it must actually be stated, or planning goes back to walking every
	// asset_scans chunk (88ms vs 16ms on preview).
	list, count := renderedCurrentLocationsQueries()
	bound := normalizeSQL(freshTailChunkBound)
	cut := normalizeSQL(freshTailCut)
	for name, q := range map[string]string{"list": list, "count": count} {
		assert.Contains(t, q, "s.timestamp >= "+bound+" AND s.timestamp >= "+cut,
			"%s query: the tail needs both the constify-able bound and the exact cut", name)
	}
}

// The count paginates the rows the list returns. If only one of them learned
// about the tail, total_count and the page disagree the moment anything is saved.
func TestCurrentLocationsQueries_ListAndCountShareOneSource(t *testing.T) {
	list, count := renderedCurrentLocationsQueries()
	latest := normalizeSQL(latestScansCTE())

	assert.Contains(t, list, latest)
	assert.Contains(t, count, latest)
}
