//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		"a collision is an update, not an insert — movement evaluation must skip it")
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
