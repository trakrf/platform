//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

// seedLocation lives in tags_conflict_integration_test.go (same package).

// seedScan inserts an asset_scans row at an explicit timestamp.
func seedScan(t *testing.T, pool *pgxpool.Pool, orgID, assetID int, locationID *int, ts time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id) VALUES ($1, $2, $3, $4)`,
		ts, orgID, assetID, locationID)
	require.NoError(t, err)
}

func TestPreviousAssetLocations_NoScansYieldsEmptyMap(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "PREV-NONE")

	prev, err := db.Store.PreviousAssetLocations(ctx, orgID, []int{asset.ID}, time.Now())
	require.NoError(t, err)
	require.Empty(t, prev, "an asset with no scans must be absent, not present-with-nil")
}

// The CAGG lags ~1-1.5 minutes and is materialized_only, so a scan written
// seconds ago is invisible to it. Without the bounded base-table tail this
// lookup would report "never seen" (inventing a move) or a stale origin
// (silently suppressing a real move). This is the case that fails silently.
func TestPreviousAssetLocations_RecentScanFoundViaBaseTail(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "PREV-TAIL")
	loc := seedLocation(t, db.AdminPool, orgID, "TAIL-A", "Receiving")

	now := time.Now()
	seedScan(t, db.AdminPool, orgID, asset.ID, &loc, now.Add(-30*time.Second))
	// Deliberately NOT refreshing the continuous aggregate: this asserts the
	// tail alone answers correctly inside the lag window.

	prev, err := db.Store.PreviousAssetLocations(ctx, orgID, []int{asset.ID}, now)
	require.NoError(t, err)
	got, ok := prev[asset.ID]
	require.True(t, ok)
	require.NotNil(t, got.LocationID)
	require.Equal(t, loc, *got.LocationID)
}

// Beyond the tail's 10-minute constant lower bound, the CAGG is the only
// source. start_offset => NULL on the refresh policy means it covers all
// history, so there is no blind spot and no lookback bound is needed.
func TestPreviousAssetLocations_OldScanFoundViaCAGG(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "PREV-CAGG")
	loc := seedLocation(t, db.AdminPool, orgID, "CAGG-A", "Yard")

	now := time.Now()
	seedScan(t, db.AdminPool, orgID, asset.ID, &loc, now.Add(-2*time.Hour))
	testutil.RefreshAssetScanLatest(t, db.AdminPool)

	prev, err := db.Store.PreviousAssetLocations(ctx, orgID, []int{asset.ID}, now)
	require.NoError(t, err)
	got, ok := prev[asset.ID]
	require.True(t, ok, "a scan older than the base-table tail must still be found via the CAGG")
	require.NotNil(t, got.LocationID)
	require.Equal(t, loc, *got.LocationID)
}

// The evaluator runs post-commit, so the row it is evaluating is already in the
// base table at exactly `before`. It must not be returned as its own predecessor.
func TestPreviousAssetLocations_ExcludesScanAtBoundary(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "PREV-BOUND")
	older := seedLocation(t, db.AdminPool, orgID, "BOUND-A", "Receiving")
	newer := seedLocation(t, db.AdminPool, orgID, "BOUND-B", "Bay 3")

	at := time.Now()
	seedScan(t, db.AdminPool, orgID, asset.ID, &older, at.Add(-time.Minute))
	seedScan(t, db.AdminPool, orgID, asset.ID, &newer, at)
	testutil.RefreshAssetScanLatest(t, db.AdminPool)

	prev, err := db.Store.PreviousAssetLocations(ctx, orgID, []int{asset.ID}, at)
	require.NoError(t, err)
	got, ok := prev[asset.ID]
	require.True(t, ok)
	require.NotNil(t, got.LocationID)
	require.Equal(t, older, *got.LocationID, "the scan at `before` is the one being evaluated, not its predecessor")
}

func TestPreviousAssetLocations_NullLocationIsASighting(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "PREV-NULL")

	now := time.Now()
	seedScan(t, db.AdminPool, orgID, asset.ID, nil, now.Add(-30*time.Second))

	prev, err := db.Store.PreviousAssetLocations(ctx, orgID, []int{asset.ID}, now)
	require.NoError(t, err)
	got, ok := prev[asset.ID]
	require.True(t, ok, "a location-less scan is still a sighting")
	require.Nil(t, got.LocationID)
}

// RLS does not extend to continuous aggregates, so the explicit org_id
// predicate on the CAGG read is the only thing keeping tenants apart. This is
// the regression guard for that.
func TestPreviousAssetLocations_CrossOrgIsolation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	orgB := createOrg(t, db.AdminPool, "Prev Org B", "prev-org-b")

	assetA := testutil.CreateTestAsset(t, db.AdminPool, orgA, "PREV-XORG")
	locA := seedLocation(t, db.AdminPool, orgA, "XORG-A", "A Dock")

	now := time.Now()
	seedScan(t, db.AdminPool, orgA, assetA.ID, &locA, now.Add(-30*time.Second))
	seedScan(t, db.AdminPool, orgA, assetA.ID, &locA, now.Add(-2*time.Hour))
	testutil.RefreshAssetScanLatest(t, db.AdminPool)

	// Org B asks about org A's asset id: both sources must come back empty.
	prev, err := db.Store.PreviousAssetLocations(ctx, orgB, []int{assetA.ID}, now)
	require.NoError(t, err)
	require.Empty(t, prev)
}

func TestPreviousAssetLocations_BatchesManyAssets(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	loc := seedLocation(t, db.AdminPool, orgID, "BATCH-A", "Staging")

	now := time.Now()
	seen := testutil.CreateTestAsset(t, db.AdminPool, orgID, "BATCH-SEEN")
	unseen := testutil.CreateTestAsset(t, db.AdminPool, orgID, "BATCH-UNSEEN")
	seedScan(t, db.AdminPool, orgID, seen.ID, &loc, now.Add(-time.Minute))

	prev, err := db.Store.PreviousAssetLocations(ctx, orgID, []int{seen.ID, unseen.ID}, now)
	require.NoError(t, err)
	require.Len(t, prev, 1)
	require.Contains(t, prev, seen.ID)
	require.NotContains(t, prev, unseen.ID)
}

func TestLookupMoveNames(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	orgB := createOrg(t, db.AdminPool, "Names Org B", "names-org-b")

	asset := testutil.CreateTestAsset(t, db.AdminPool, orgA, "NAME-1")
	loc := seedLocation(t, db.AdminPool, orgA, "NAME-LOC", "Bay 3")

	assets, locations, err := db.Store.LookupMoveNames(ctx, orgA, []int{asset.ID}, []int{loc})
	require.NoError(t, err)
	require.Equal(t, "NAME-1", assets[asset.ID].ExternalKey)
	require.NotEmpty(t, assets[asset.ID].Name)
	require.Equal(t, "Bay 3", locations[loc])

	// Another org's ids resolve to nothing.
	assets, locations, err = db.Store.LookupMoveNames(ctx, orgB, []int{asset.ID}, []int{loc})
	require.NoError(t, err)
	require.Empty(t, assets)
	require.Empty(t, locations)
}
