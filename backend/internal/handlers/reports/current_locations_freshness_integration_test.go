//go:build integration
// +build integration

// TRA-1117: a scan must be visible on /reports/asset-locations the moment it
// commits, not once the asset_scan_latest continuous aggregate next refreshes.
//
// The CAGG is materialized_only (real-time aggregation is incompatible with the
// RLS on asset_scans), so a CAGG-only read lags by end_offset + schedule_interval
// — up to ~2 minutes. The report therefore unions a bounded tail of raw
// asset_scans into the same shape as the CAGG.
//
// Every test in this file deliberately does NOT call
// testutil.RefreshAssetScanLatest for the rows whose freshness is under test:
// that omission IS the test. The other reports suites still refresh, and must
// keep passing unchanged — the tail adds visibility, it must not change any
// answer the CAGG already had.

package reports

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

// fetchCurrentLocations runs the report and returns the decoded envelope, so
// each test can assert on both the rows and total_count — the list and the
// count are separate queries and TRA-1117 has to fix both.
func fetchCurrentLocations(t *testing.T, h *Handler, orgID int) currLocResp {
	t.Helper()
	router := setupTemporalReportsRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/asset-locations", nil)
	req = withReportsOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp currLocResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// seedSecondOrgForReports creates a neighbouring org. testutil.CreateTestAccount
// hardcodes the "test-org" identifier, so a second call collides on the unique
// index — cross-org tests seed their own the same way elsewhere in the suite.
func seedSecondOrgForReports(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var id int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.organizations (name, identifier, is_active)
		VALUES ('Neighbour Org', 'neighbour-org', true) RETURNING id
	`).Scan(&id))
	return id
}

// The headline bug: save from the Scan tab, look at Reports, see nothing. The
// scan is committed and the CAGG has not refreshed yet. Both the rows and
// total_count must already reflect it.
func TestListCurrentLocations_UnmaterializedScanIsVisibleImmediately(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)

	asset := seedAssetForReports(t, pool, orgID, "FRESH-A1", dayAgo, nil)
	loc := seedLocationForReports(t, pool, orgID, "FRESH-L1", dayAgo, nil)

	// The save. No CAGG refresh — this is exactly the state the demo hits.
	seedScan(t, pool, orgID, asset, loc, now)

	resp := fetchCurrentLocations(t, NewHandler(store), orgID)

	require.Len(t, resp.Data, 1, "an unmaterialized scan must still produce a row")
	assert.Equal(t, 1, resp.TotalCount, "CountCurrentLocations must see the tail too")

	item := resp.Data[0]
	assert.Equal(t, "FRESH-A1", item.AssetExternalKey)
	require.NotNil(t, item.LocationExternalKey)
	assert.Equal(t, "FRESH-L1", *item.LocationExternalKey)
	assert.WithinDuration(t, now, item.AssetLastSeen.Time, time.Second,
		"last_seen must be the scan just written, not a materialized predecessor")
}

// A scan inside the tail window that the CAGG has ALSO already materialized
// arrives down both branches of the union. It must collapse to exactly one row
// with the same answer the CAGG-only path gave — that idempotence is what makes
// the overlap safe to leave unbounded on the old side.
func TestListCurrentLocations_TailOverlappingCAGG_ResolvesToOneRow(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)

	asset := seedAssetForReports(t, pool, orgID, "DUP-A1", dayAgo, nil)
	loc := seedLocationForReports(t, pool, orgID, "DUP-L1", dayAgo, nil)

	scanAt := now.Add(-3 * time.Minute)
	seedScan(t, pool, orgID, asset, loc, scanAt)

	// Now the row exists in the CAGG *and* inside the raw tail window.
	testutil.RefreshAssetScanLatest(t, pool)

	resp := fetchCurrentLocations(t, NewHandler(store), orgID)

	require.Len(t, resp.Data, 1, "a row present in both the CAGG and the tail must not double up")
	assert.Equal(t, 1, resp.TotalCount)

	item := resp.Data[0]
	require.NotNil(t, item.LocationExternalKey)
	assert.Equal(t, "DUP-L1", *item.LocationExternalKey)
	assert.WithinDuration(t, scanAt, item.AssetLastSeen.Time, time.Second)
}

// A move recorded only in the tail must win over the materialized location. Read
// through the CAGG alone this asset is still at its old location; that is the
// "shows the previous save" symptom, expressed at the storage layer.
func TestListCurrentLocations_UnmaterializedMoveBeatsMaterializedLocation(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)

	asset := seedAssetForReports(t, pool, orgID, "MOVE-A1", dayAgo, nil)
	oldLoc := seedLocationForReports(t, pool, orgID, "MOVE-L-OLD", dayAgo, nil)
	newLoc := seedLocationForReports(t, pool, orgID, "MOVE-L-NEW", dayAgo, nil)

	seedScan(t, pool, orgID, asset, oldLoc, now.Add(-30*time.Minute))
	testutil.RefreshAssetScanLatest(t, pool)

	// The save that has not been materialized yet.
	seedScan(t, pool, orgID, asset, newLoc, now)

	resp := fetchCurrentLocations(t, NewHandler(store), orgID)

	require.Len(t, resp.Data, 1)
	item := resp.Data[0]
	require.NotNil(t, item.LocationExternalKey)
	assert.Equal(t, "MOVE-L-NEW", *item.LocationExternalKey,
		"the unmaterialized scan is the latest and must win")
}

// Dwell reads the same source, so a fresh scan has to extend the current run
// without disturbing where that run started. dwell_started_at still comes out of
// materialized history (the run began long before the tail window); only
// last_seen — and therefore dwell_seconds — moves.
func TestDwell_UnmaterializedScanExtendsRunWithoutMovingItsStart(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	base := dwellBase() // minute-aligned, two hours back

	asset := seedAssetForReports(t, pool, orgID, "FDW-A1", base.Add(-24*time.Hour), nil)
	oldLoc := seedLocationForReports(t, pool, orgID, "FDW-L-OLD", base.Add(-24*time.Hour), nil)
	loc := seedLocationForReports(t, pool, orgID, "FDW-L-RUN", base.Add(-24*time.Hour), nil)

	// Somewhere else first, so the run start is a real gaps-and-islands answer
	// rather than the fall-through-to-first-bucket case.
	seedScan(t, pool, orgID, asset, oldLoc, base)
	seedScan(t, pool, orgID, asset, loc, base.Add(20*time.Minute))
	seedScan(t, pool, orgID, asset, loc, base.Add(30*time.Minute))
	testutil.RefreshAssetScanLatest(t, pool)

	// Same location, just now, unmaterialized.
	fresh := time.Now().UTC().Truncate(time.Minute)
	seedScan(t, pool, orgID, asset, loc, fresh)

	resp := fetchCurrentLocations(t, NewHandler(store), orgID)
	require.Len(t, resp.Data, 1)
	item := resp.Data[0]

	require.NotNil(t, item.LocationExternalKey)
	assert.Equal(t, "FDW-L-RUN", *item.LocationExternalKey)
	assert.WithinDuration(t, base.Add(20*time.Minute), item.DwellStartedAt.Time, time.Second,
		"the run start is materialized history and must be unaffected by the tail")
	assert.WithinDuration(t, fresh, item.AssetLastSeen.Time, time.Second)
	assert.Equal(t, int64(fresh.Sub(base.Add(20*time.Minute))/time.Second), item.DwellSeconds,
		"dwell must extend to the fresh scan, not stop at the last materialized one")
}

// The list and the count are two separately-built queries. Both must read the
// same source, or pagination reports a total that the rows cannot reach.
func TestCountCurrentLocations_AgreesWithList_AcrossTailAndCAGG(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)

	materialized := seedAssetForReports(t, pool, orgID, "MIX-A-OLD", dayAgo, nil)
	tailOnly := seedAssetForReports(t, pool, orgID, "MIX-A-NEW", dayAgo, nil)
	loc := seedLocationForReports(t, pool, orgID, "MIX-L1", dayAgo, nil)

	seedScan(t, pool, orgID, materialized, loc, now.Add(-30*time.Minute))
	testutil.RefreshAssetScanLatest(t, pool)
	seedScan(t, pool, orgID, tailOnly, loc, now)

	resp := fetchCurrentLocations(t, NewHandler(store), orgID)

	assert.Len(t, resp.Data, 2, "one materialized asset plus one visible only via the tail")
	assert.Equal(t, 2, resp.TotalCount, "count must not disagree with the rows it paginates")

	keys := make([]string, 0, len(resp.Data))
	for _, item := range resp.Data {
		keys = append(keys, item.AssetExternalKey)
	}
	assert.ElementsMatch(t, []string{"MIX-A-OLD", "MIX-A-NEW"}, keys)
}

// A scan older than the tail window must come from the CAGG alone. If the tail
// were unbounded (or the bound wrong-signed) this would still pass while the
// query silently scanned all of asset_scans — so the assertion here is that the
// CAGG remains load-bearing: without a refresh, an old scan is NOT visible.
func TestListCurrentLocations_ScanOlderThanTail_StillNeedsTheCAGG(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)

	asset := seedAssetForReports(t, pool, orgID, "OLD-A1", dayAgo, nil)
	loc := seedLocationForReports(t, pool, orgID, "OLD-L1", dayAgo, nil)

	// Well outside the tail window, and deliberately never materialized.
	seedScan(t, pool, orgID, asset, loc, now.Add(-2*time.Hour))

	resp := fetchCurrentLocations(t, NewHandler(store), orgID)
	assert.Empty(t, resp.Data, "the tail is bounded; history still comes from the CAGG")
	assert.Equal(t, 0, resp.TotalCount)

	// And once materialized it appears, unchanged by the tail.
	testutil.RefreshAssetScanLatest(t, pool)
	resp = fetchCurrentLocations(t, NewHandler(store), orgID)
	require.Len(t, resp.Data, 1)
	require.NotNil(t, resp.Data[0].LocationExternalKey)
	assert.Equal(t, "OLD-L1", *resp.Data[0].LocationExternalKey)
}

// The tail read runs inside WithOrgTx against the RLS-guarded asset_scans, and
// carries an explicit org_id filter besides. A fresh scan in another org must
// not leak into this org's report — the CAGG branch has no RLS at all, so the
// explicit filter is the only thing standing between the two on that side.
func TestListCurrentLocations_TailDoesNotLeakAcrossOrgs(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgA := testutil.CreateTestAccount(t, pool)
	orgB := seedSecondOrgForReports(t, pool)
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)

	assetB := seedAssetForReports(t, pool, orgB, "LEAK-A-B", dayAgo, nil)
	locB := seedLocationForReports(t, pool, orgB, "LEAK-L-B", dayAgo, nil)
	seedScan(t, pool, orgB, assetB, locB, now)

	resp := fetchCurrentLocations(t, NewHandler(store), orgA)
	assert.Empty(t, resp.Data, "org A must not see org B's fresh scan")
	assert.Equal(t, 0, resp.TotalCount)

	respB := fetchCurrentLocations(t, NewHandler(store), orgB)
	require.Len(t, respB.Data, 1, "org B still sees its own")
}

// Guard against a tail read that forgets the asset_scans RLS context. The tail
// is the first raw-hypertable read in this query; if it were ever moved outside
// WithOrgTx the policy qual aborts the scan rather than returning nothing, so
// pin that the endpoint stays 200 with a fresh row present.
func TestListCurrentLocations_TailReadRunsInsideOrgContext(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	now := time.Now().UTC()

	var rlsEnabled bool
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT relrowsecurity FROM pg_class
		WHERE oid = 'trakrf.asset_scans'::regclass
	`).Scan(&rlsEnabled))
	require.True(t, rlsEnabled, "precondition: asset_scans is RLS-guarded")

	asset := seedAssetForReports(t, pool, orgID, "RLS-A1", now.Add(-24*time.Hour), nil)
	loc := seedLocationForReports(t, pool, orgID, "RLS-L1", now.Add(-24*time.Hour), nil)
	seedScan(t, pool, orgID, asset, loc, now)

	resp := fetchCurrentLocations(t, NewHandler(store), orgID)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "RLS-A1", resp.Data[0].AssetExternalKey)
}
