//go:build integration

package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/scanread"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// TRA-1118: with timestamps truncated to the minute, a manual save can collide
// with a fixed-reader row for the same asset in the same minute. The save must
// upsert (operator beats reader), not raise a unique violation — pre-fix this
// 500'd the whole save transaction.
func TestSaveInventoryScans_CollidesWithReaderRowUpdatesInPlace(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerDevice(t, db, orgID, "cs463-a")
	registerRFIDTag(t, db, orgID, testEPC)

	var readerLoc, saveLoc int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, 'dock', 'Dock') RETURNING id`,
		orgID).Scan(&readerLoc))
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, 'shelf', 'Shelf') RETURNING id`,
		orgID).Scan(&saveLoc))
	_, err := db.AdminPool.Exec(ctx,
		`UPDATE trakrf.scan_points SET location_id = $1 WHERE org_id = $2 AND scan_device_id = $3`,
		readerLoc, orgID, dev.ID)
	require.NoError(t, err)

	// Fixed reader stores this minute's row for the asset. PersistReads
	// truncates time.Now() to the same minute floor SaveInventoryScans uses,
	// guaranteeing the collision this test exists to pin. (A boundary-straddling
	// flake is impossible: both paths call time.Now() microseconds apart, and
	// even if they did straddle a minute, the save would land its own row and
	// the count/no-error assertions still hold — the test just wouldn't pin the
	// collision. The single-row assertion below is what proves it collided.)
	res, err := db.Store.PersistReads(ctx, orgID, dev.ID, 1, time.Now(),
		[]scanread.Read{{EPC: testEPC, AntennaPort: 1, RSSI: -56}})
	require.NoError(t, err)
	require.Equal(t, 1, res.Inserted)
	var assetID int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT asset_id FROM trakrf.asset_scans WHERE org_id = $1`, orgID).Scan(&assetID))

	// Operator saves the same asset at a different location in the same minute.
	result, err := db.Store.SaveInventoryScans(ctx, orgID, storage.SaveInventoryRequest{
		LocationID: saveLoc,
		AssetIDs:   []int{assetID},
	})
	require.NoError(t, err, "same-minute collision must upsert, not raise a unique violation")
	assert.Equal(t, 1, result.Count)
	assert.Empty(t, result.InsertedAssetIDs,
		"a collision is an update, not an insert — the normal evaluation path must skip it")
	require.Contains(t, result.OverriddenFrom, assetID,
		"a location-changing override must carry the captured pre-save origin")
	require.NotNil(t, result.OverriddenFrom[assetID])
	assert.Equal(t, readerLoc, *result.OverriddenFrom[assetID],
		"the origin is the reader's location the save overrode")
	assert.True(t, result.Timestamp.Equal(result.Timestamp.Truncate(time.Minute)),
		"save timestamp is minute-truncated")

	// Still one row for the minute; the operator's location won, and the row is
	// coherently a manual observation (scan_point/tag_scan cleared).
	var n int
	var gotLoc int
	var scanPointID, tagScanID *int64
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.asset_scans WHERE org_id = $1`, orgID).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT location_id, scan_point_id, tag_scan_id FROM trakrf.asset_scans WHERE org_id = $1`,
		orgID).Scan(&gotLoc, &scanPointID, &tagScanID))
	assert.Equal(t, saveLoc, gotLoc, "operator's explicit save beats the passive reader read")
	assert.Nil(t, scanPointID)
	assert.Nil(t, tagScanID)
}

// saveFixture creates an org with two locations and two assets, and returns
// the ids the save tests need.
type saveFixture struct {
	orgID, locA, locB, asset1, asset2 int
}

func newSaveFixture(t *testing.T, db *testutil.TestDB) saveFixture {
	t.Helper()
	ctx := context.Background()
	f := saveFixture{orgID: testutil.CreateTestAccount(t, db.AdminPool)}
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, 'a', 'A') RETURNING id`,
		f.orgID).Scan(&f.locA))
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, 'b', 'B') RETURNING id`,
		f.orgID).Scan(&f.locB))
	f.asset1 = testutil.CreateTestAsset(t, db.AdminPool, f.orgID, "asset-1").ID
	f.asset2 = testutil.CreateTestAsset(t, db.AdminPool, f.orgID, "asset-2").ID
	return f
}

func TestSaveInventoryScansRecord_HookSeesInsertedAndOverridden(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	f := newSaveFixture(t, db)

	// asset2 already has this minute's row at A, so saving it at B is an
	// override; asset1 has no row, so its save is an insert. Both time.Now()
	// calls are microseconds apart, as in the collision test above.
	_, err := db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id) VALUES ($1, $2, $3, $4)`,
		time.Now().Truncate(storage.ScanGranularity), f.orgID, f.asset2, f.locA)
	require.NoError(t, err)

	var got storage.SavedInventory
	var hookOrg int
	calls := 0
	hook := func(ctx context.Context, tx pgx.Tx, saved storage.SavedInventory) error {
		calls++
		got = saved
		// The org GUC proves the hook is on the save's own transaction.
		return tx.QueryRow(ctx, `SELECT current_setting('app.current_org_id')::bigint`).Scan(&hookOrg)
	}
	result, err := db.Store.SaveInventoryScansRecord(ctx, f.orgID, storage.SaveInventoryRequest{
		LocationID: f.locB,
		AssetIDs:   []int{f.asset1, f.asset2},
	}, hook)
	require.NoError(t, err)

	require.Equal(t, 1, calls)
	assert.Equal(t, f.orgID, hookOrg)
	assert.Equal(t, f.orgID, got.OrgID)
	assert.Equal(t, f.locB, got.LocationID)
	assert.Equal(t, result.Timestamp, got.Timestamp)
	assert.True(t, got.Timestamp.Equal(got.Timestamp.Truncate(storage.ScanGranularity)), "the saved minute")
	assert.Equal(t, []int{f.asset1}, got.InsertedAssetIDs)
	require.Contains(t, got.OverriddenFrom, f.asset2)
	require.NotNil(t, got.OverriddenFrom[f.asset2])
	assert.Equal(t, f.locA, *got.OverriddenFrom[f.asset2])
}

func TestSaveInventoryScansRecord_HookErrorKeepsSave(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	f := newSaveFixture(t, db)

	hook := func(ctx context.Context, tx pgx.Tx, saved storage.SavedInventory) error {
		_, _, err := db.Store.InsertLocationChangeIfAbsentTx(ctx, tx, saved.OrgID, notificationevent.LocationChange{
			OrgID: saved.OrgID, AssetID: f.asset1, ToLocationID: saved.LocationID,
			Origin: notificationevent.OriginManualSave, DetectedAt: saved.Timestamp,
		})
		require.NoError(t, err)
		return errors.New("hook exploded after writing")
	}
	result, err := db.Store.SaveInventoryScansRecord(ctx, f.orgID, storage.SaveInventoryRequest{
		LocationID: f.locB,
		AssetIDs:   []int{f.asset1},
	}, hook)
	require.NoError(t, err, "a hook error never fails the save")
	assert.Equal(t, 1, result.Count)

	var scans, changes int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.asset_scans WHERE org_id = $1`, f.orgID).Scan(&scans))
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.location_changes WHERE org_id = $1`, f.orgID).Scan(&changes))
	assert.Equal(t, 1, scans, "the scan row committed")
	assert.Zero(t, changes, "the hook's own write rolled back to the savepoint")
}

func TestSaveInventoryScansRecord_NilHookIsSaveInventoryScans(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	f := newSaveFixture(t, db)

	result, err := db.Store.SaveInventoryScansRecord(ctx, f.orgID, storage.SaveInventoryRequest{
		LocationID: f.locA,
		AssetIDs:   []int{f.asset1},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, []int{f.asset1}, result.InsertedAssetIDs)
}

func TestSaveInventoryScansRecord_HookWriteSurvivesRelease(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	f := newSaveFixture(t, db)

	hook := func(ctx context.Context, tx pgx.Tx, saved storage.SavedInventory) error {
		_, inserted, err := db.Store.InsertLocationChangeIfAbsentTx(ctx, tx, saved.OrgID, notificationevent.LocationChange{
			OrgID: saved.OrgID, AssetID: f.asset1, ToLocationID: saved.LocationID,
			Origin: notificationevent.OriginManualSave, DetectedAt: saved.Timestamp,
		})
		require.NoError(t, err)
		require.True(t, inserted)
		return nil
	}
	_, err := db.Store.SaveInventoryScansRecord(ctx, f.orgID, storage.SaveInventoryRequest{
		LocationID: f.locB,
		AssetIDs:   []int{f.asset1},
	}, hook)
	require.NoError(t, err)

	var scans, changes int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.asset_scans WHERE org_id = $1`, f.orgID).Scan(&scans))
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.location_changes WHERE org_id = $1`, f.orgID).Scan(&changes))
	assert.Equal(t, 1, scans)
	assert.Equal(t, 1, changes, "the hook's write committed with the save")
}

func TestSaveInventoryScansRecord_HookPanicKeepsSave(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	f := newSaveFixture(t, db)

	hook := func(ctx context.Context, tx pgx.Tx, saved storage.SavedInventory) error {
		var m map[int]int
		m[f.asset1] = saved.LocationID // nil map write: the kind of bug a recorder could have
		return nil
	}
	var result *storage.SaveInventoryResult
	var err error
	require.NotPanics(t, func() {
		result, err = db.Store.SaveInventoryScansRecord(ctx, f.orgID, storage.SaveInventoryRequest{
			LocationID: f.locB,
			AssetIDs:   []int{f.asset1},
		}, hook)
	})
	require.NoError(t, err, "a hook panic never fails the save")
	assert.Equal(t, 1, result.Count)
	assert.Equal(t, []int{f.asset1}, result.InsertedAssetIDs)

	var scans int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.asset_scans WHERE org_id = $1`, f.orgID).Scan(&scans))
	assert.Equal(t, 1, scans, "the scan row committed")
}

func TestSaveInventoryScansRecord_UnchangedSaveSkipsHook(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	f := newSaveFixture(t, db)

	calls := 0
	hook := func(context.Context, pgx.Tx, storage.SavedInventory) error { calls++; return nil }
	req := storage.SaveInventoryRequest{LocationID: f.locA, AssetIDs: []int{f.asset1}}
	_, err := db.Store.SaveInventoryScansRecord(ctx, f.orgID, req, hook)
	require.NoError(t, err)
	require.Equal(t, 1, calls, "the first save inserted a row")

	// Same minute, same location: neither inserted nor overridden.
	result, err := db.Store.SaveInventoryScansRecord(ctx, f.orgID, req, hook)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Count)
	assert.Empty(t, result.InsertedAssetIDs)
	assert.Empty(t, result.OverriddenFrom)
	assert.Equal(t, 1, calls, "nothing to record, so the hook is not called")
}
