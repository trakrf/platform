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
			"%s query must union the raw tail or the report lags materialization", name)
		assert.Contains(t, q, "FROM trakrf.asset_scan_latest",
			"%s query must still take history from the CAGG", name)
	}
}

// The dwell LATERAL is the third consumer, and the easiest to forget: miss it and
// locations are fresh while dwell is stale or null on exactly the row the user
// just created.
func TestCurrentLocationsQuery_DwellReadsTheSameSource(t *testing.T) {
	list, _ := renderedCurrentLocationsQueries()

	assert.Contains(t, list, "FROM scan_source r")
	assert.Contains(t, list, "FROM scan_source c")
	assert.NotContains(t, list, "FROM trakrf.asset_scan_latest r",
		"the dwell walk-back must go through scan_source, not the bare CAGG")
	assert.NotContains(t, list, "FROM trakrf.asset_scan_latest c",
		"the dwell walk-back must go through scan_source, not the bare CAGG")
}

// scan_source is referenced by both legs of the dwell LATERAL, so PostgreSQL
// would materialize it by default — aggregating the org's whole history before
// the LIMIT applies, then re-scanning it per page row. Inlining is what keeps
// each dwell probe an index lookup.
func TestCurrentLocationsQuery_ScanSourceIsInlined(t *testing.T) {
	list, _ := renderedCurrentLocationsQueries()

	require.Contains(t, list, "scan_source AS NOT MATERIALIZED (")
	assert.Equal(t, 2, strings.Count(list, "FROM scan_source "),
		"if scan_source picks up more references, re-check the plan before trusting NOT MATERIALIZED")
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

// The two sources must be split at the same instant on both sides of the cut,
// and that instant must be a bucket boundary. An overlapping or gapped partition
// either double-counts a bucket into the dwell walk-back or drops one entirely.
func TestCurrentLocationsQueries_PartitionAtOneBucketAlignedCut(t *testing.T) {
	list, count := renderedCurrentLocationsQueries()
	cut := normalizeSQL(scanSourceCut)

	require.Contains(t, cut, "time_bucket(INTERVAL '1 minute'",
		"the cut must land on a bucket boundary for the partition to be exact")
	require.Contains(t, cut, freshScanTailWindow)

	for name, q := range map[string]string{"list": list, "count": count} {
		assert.Contains(t, q, "bucket < "+cut, "%s query: CAGG branch takes settled history only", name)
		assert.Contains(t, q, "s.timestamp >= "+cut, "%s query: raw branch takes the tail only", name)
		assert.Equal(t, strings.Count(q, "bucket < "+cut), strings.Count(q, "s.timestamp >= "+cut),
			"%s query: every CAGG branch needs its matching tail branch", name)
	}
}

// The tail must be wider than the worst-case materialization lag — end_offset
// (1 min) + schedule_interval (30s) + a slipped cycle. Narrowing it below that
// silently reopens the invisibility window with no other symptom.
func TestFreshScanTailWindow_ClearsTheMaterializationLag(t *testing.T) {
	assert.Equal(t, "5 minutes", freshScanTailWindow,
		"changing this is a freshness decision, not a tuning knob; see 000028_asset_scan_latest_policy")
}

// The count paginates the rows the list returns. If only one of them learned
// about the tail, total_count and the page disagree the moment anything is saved.
func TestCurrentLocationsQueries_ListAndCountShareOneSource(t *testing.T) {
	list, count := renderedCurrentLocationsQueries()
	latest := normalizeSQL(latestScansCTE())

	assert.Contains(t, list, latest)
	assert.Contains(t, count, latest)
}
