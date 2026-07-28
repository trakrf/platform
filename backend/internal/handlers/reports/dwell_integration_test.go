//go:build integration
// +build integration

// TRA-1023: dwell (presence interval) on /reports/asset-locations. Each item
// reports dwell_started_at — the first observation of the asset at its current
// location within the current unbroken run — and dwell_seconds, the OBSERVED
// span asset_last_seen - dwell_started_at.
//
// The run is derived by gaps-and-islands over the asset_scan_latest continuous
// aggregate (TRA-1022), which is materialized-only: every test here must call
// testutil.RefreshAssetScanLatest after seeding or the report comes back empty.

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

	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// seedScanNoLocation records a scan that resolved to no location. asset_scans
// .location_id is nullable (000008); a NULL-location scan is a distinct
// presence state, not a continuation of the previous one.
func seedScanNoLocation(t *testing.T, pool *pgxpool.Pool, orgID, assetID int, ts time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO trakrf.asset_scans (org_id, asset_id, location_id, timestamp)
		VALUES ($1, $2, NULL, $3)
	`, orgID, assetID, ts)
	require.NoError(t, err)
}

// dwellBase returns a minute-aligned instant two hours in the past. Alignment
// keeps each seeded scan in its own 1-minute CAGG bucket, which is the
// resolution at which a location change is observable at all. Two hours ago
// (rather than an arbitrarily old timestamp) stays clear of the 365-day
// asset_scans retention policy that reaps stale-stamped test rows.
func dwellBase() time.Time {
	return time.Now().UTC().Truncate(time.Minute).Add(-2 * time.Hour)
}

// fetchOneDwellRow runs the report and returns the single expected item.
func fetchOneDwellRow(t *testing.T, h *Handler, orgID int) report.PublicCurrentLocationItem {
	t.Helper()
	router := setupTemporalReportsRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/reports/asset-locations", nil)
	req = withReportsOrg(req, orgID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp currLocResp
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1, "exactly one asset expected; body: %s", w.Body.String())
	return resp.Data[0]
}

// An asset that moved must dwell from the start of its CURRENT run, not from
// its first scan ever. This is the whole point of the gaps-and-islands pass —
// a plain min(timestamp) would report the earlier location's arrival.
func TestDwell_StartsAtRunStart_NotFirstScanEver(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	base := dwellBase()

	asset := seedAssetForReports(t, pool, orgID, "DW-A1", base.Add(-24*time.Hour), nil)
	oldLoc := seedLocationForReports(t, pool, orgID, "DW-L-OLD", base.Add(-24*time.Hour), nil)
	newLoc := seedLocationForReports(t, pool, orgID, "DW-L-NEW", base.Add(-24*time.Hour), nil)

	// At the old location, then moved: the run starts at base+20m.
	seedScan(t, pool, orgID, asset, oldLoc, base)
	seedScan(t, pool, orgID, asset, oldLoc, base.Add(10*time.Minute))
	seedScan(t, pool, orgID, asset, newLoc, base.Add(20*time.Minute))
	seedScan(t, pool, orgID, asset, newLoc, base.Add(25*time.Minute))
	seedScan(t, pool, orgID, asset, newLoc, base.Add(30*time.Minute))
	testutil.RefreshAssetScanLatest(t, pool)

	item := fetchOneDwellRow(t, NewHandler(store), orgID)

	require.NotNil(t, item.LocationExternalKey)
	assert.Equal(t, "DW-L-NEW", *item.LocationExternalKey)
	assert.WithinDuration(t, base.Add(20*time.Minute), item.DwellStartedAt.Time, time.Second,
		"dwell must start when the asset arrived at DW-L-NEW, not at its first scan ever")
	assert.Equal(t, int64(600), item.DwellSeconds,
		"dwell spans arrival (base+20m) to last seen (base+30m)")
}

// An asset that has never been anywhere else dwells from its first bucket ever.
// This is the walk-back's terminating case: no differing location exists, so
// the search COALESCEs to -infinity and falls through to the earliest bucket.
func TestDwell_StationaryAsset_DwellsFromFirstScanEver(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	base := dwellBase()

	asset := seedAssetForReports(t, pool, orgID, "DW-A2", base.Add(-24*time.Hour), nil)
	loc := seedLocationForReports(t, pool, orgID, "DW-L-ONLY", base.Add(-24*time.Hour), nil)

	seedScan(t, pool, orgID, asset, loc, base)
	seedScan(t, pool, orgID, asset, loc, base.Add(15*time.Minute))
	seedScan(t, pool, orgID, asset, loc, base.Add(30*time.Minute))
	testutil.RefreshAssetScanLatest(t, pool)

	item := fetchOneDwellRow(t, NewHandler(store), orgID)

	assert.WithinDuration(t, base, item.DwellStartedAt.Time, time.Second,
		"a never-moved asset dwells from its earliest observation")
	assert.Equal(t, int64(1800), item.DwellSeconds)
}

// Dwell is an OBSERVED span, not a wall clock: it is measured to
// asset_last_seen, so it freezes when reads stop rather than growing forever
// for an asset that has left. The seeded scans stop ~90 minutes before now, so
// a now()-based implementation would report ~5400s+ instead of 1800s.
func TestDwell_FreezesAtLastSeen_NotWallClock(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	base := dwellBase()

	asset := seedAssetForReports(t, pool, orgID, "DW-A3", base.Add(-24*time.Hour), nil)
	loc := seedLocationForReports(t, pool, orgID, "DW-L-GONE", base.Add(-24*time.Hour), nil)

	seedScan(t, pool, orgID, asset, loc, base)
	seedScan(t, pool, orgID, asset, loc, base.Add(30*time.Minute))
	testutil.RefreshAssetScanLatest(t, pool)

	item := fetchOneDwellRow(t, NewHandler(store), orgID)

	assert.Equal(t, int64(1800), item.DwellSeconds,
		"dwell must equal last_seen - dwell_started_at, not now() - dwell_started_at")

	sinceStart := time.Since(item.DwellStartedAt.Time)
	assert.Greater(t, sinceStart, 80*time.Minute,
		"sanity: the run started well over an hour ago, so a wall-clock implementation would differ loudly")
}

// A scan that resolved to no location is a distinct presence state and breaks
// the run. Returning to the same location afterwards starts a NEW interval —
// this is what `IS DISTINCT FROM` buys over a plain `<>`, which would treat the
// NULL bucket as non-matching and silently span across it.
func TestDwell_NullLocationScanBreaksTheRun(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	base := dwellBase()

	asset := seedAssetForReports(t, pool, orgID, "DW-A4", base.Add(-24*time.Hour), nil)
	loc := seedLocationForReports(t, pool, orgID, "DW-L-BACK", base.Add(-24*time.Hour), nil)

	seedScan(t, pool, orgID, asset, loc, base)
	seedScan(t, pool, orgID, asset, loc, base.Add(10*time.Minute))
	seedScanNoLocation(t, pool, orgID, asset, base.Add(20*time.Minute))
	seedScan(t, pool, orgID, asset, loc, base.Add(30*time.Minute))
	seedScan(t, pool, orgID, asset, loc, base.Add(40*time.Minute))
	testutil.RefreshAssetScanLatest(t, pool)

	item := fetchOneDwellRow(t, NewHandler(store), orgID)

	require.NotNil(t, item.LocationExternalKey)
	assert.Equal(t, "DW-L-BACK", *item.LocationExternalKey)
	assert.WithinDuration(t, base.Add(30*time.Minute), item.DwellStartedAt.Time, time.Second,
		"the NULL-location bucket ends the earlier run; dwell restarts on return")
	assert.Equal(t, int64(600), item.DwellSeconds)
}

// Dwell describes the underlying scan run, so it survives the location entity
// being soft-deleted — exactly as asset_last_seen does. The row projects
// location_id/location_external_key as null (reports hide tombstoned anchor
// points) while still reporting how long the asset has been there.
func TestDwell_SurvivesSoftDeletedLocationProjection(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	base := dwellBase()

	asset := seedAssetForReports(t, pool, orgID, "DW-A5", base.Add(-24*time.Hour), nil)
	loc := seedLocationForReports(t, pool, orgID, "DW-L-DEL", base.Add(-24*time.Hour), nil)

	seedScan(t, pool, orgID, asset, loc, base)
	seedScan(t, pool, orgID, asset, loc, base.Add(30*time.Minute))
	testutil.RefreshAssetScanLatest(t, pool)

	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.locations SET deleted_at = now() WHERE id = $1`, loc)
	require.NoError(t, err)

	item := fetchOneDwellRow(t, NewHandler(store), orgID)

	assert.Nil(t, item.LocationID, "soft-deleted location is projected as null")
	assert.Nil(t, item.LocationExternalKey)
	assert.WithinDuration(t, base, item.DwellStartedAt.Time, time.Second,
		"dwell tracks the scan run, not the projectability of the location entity")
	assert.Equal(t, int64(1800), item.DwellSeconds)
}
