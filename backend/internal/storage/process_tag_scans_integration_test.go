//go:build integration

package storage_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanpoint"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// cs463Fixture is the representative CS463 read payload captured per TRA-899.
func cs463Fixture(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../testutil/fixtures/cs463_read.json")
	require.NoError(t, err)
	return string(b)
}

// TRA-899: process_tag_scans no longer auto-creates scan_devices/scan_points.
// A read whose device + capture point are CRUD-registered still lands in
// asset_scans; a read from an unregistered device creates no device/point/scan.

func TestProcessTagScans_RegisteredDeviceProducesAssetScan(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool) // identifier: test-org

	// A zone location for the boundary capture point.
	var locID int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1,'dock','Dock') RETURNING id`, orgID).Scan(&locID))

	// Register the device + capture point matching the fixture's
	// rfidReaderName (cs463-214) and capturePointName (cs463-214-1).
	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		ExternalKey: "cs463-214", Name: "Dock Reader", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)
	port := 1
	boundary := true
	_, err = db.Store.CreateScanPoint(ctx, orgID, dev.ID, scanpoint.CreateScanPointRequest{
		ExternalKey: "cs463-214-1", Name: "Antenna 1", AntennaPort: &port, LocationID: &locID, IsBoundary: &boundary,
	})
	require.NoError(t, err)

	// Ingest a raw scan (superuser insert; the AFTER trigger runs the parser).
	// Stamp the read at "now": the fixture's captured timeStampOfRead is from
	// 2024, and asset_scans has a 365-day retention policy whose background
	// worker would reap that chunk moments after insert (flaky 0-row reads).
	// Production reads carry current timestamps, so this matches reality.
	_, err = db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.tag_scans (message_topic, message_data)
		 VALUES ($1, jsonb_set($2::jsonb, '{tags,0,timeStampOfRead}',
		         to_jsonb(((extract(epoch FROM now()) * 1000000)::BIGINT)::text)))`,
		"test-org/cs463-214/reads", cs463Fixture(t))
	require.NoError(t, err)

	var scans int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.asset_scans WHERE org_id = $1`, orgID).Scan(&scans))
	require.Equal(t, 1, scans, "registered device read should produce exactly one asset_scan")

	// And it resolved to the registered scan_point.
	var spExternalKey string
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		SELECT sp.external_key FROM trakrf.asset_scans a
		JOIN trakrf.scan_points sp ON sp.id = a.scan_point_id
		WHERE a.org_id = $1`, orgID).Scan(&spExternalKey))
	require.Equal(t, "cs463-214-1", spExternalKey)
}

func TestProcessTagScans_UnregisteredDeviceCreatesNothing(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool) // identifier: test-org

	// No device/point registered for this org.
	_, err := db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.tag_scans (message_topic, message_data) VALUES ($1, $2::jsonb)`,
		"test-org/cs463-214/reads", cs463Fixture(t))
	require.NoError(t, err)

	var devices, points, scans int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.scan_devices WHERE org_id=$1`, orgID).Scan(&devices))
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.scan_points WHERE org_id=$1`, orgID).Scan(&points))
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.asset_scans WHERE org_id=$1`, orgID).Scan(&scans))
	require.Equal(t, 0, devices, "auto-create of scan_devices was removed (TRA-899)")
	require.Equal(t, 0, points, "auto-create of scan_points was removed (TRA-899)")
	require.Equal(t, 0, scans, "no registered capture point -> no asset_scan")

	// Asset/tag auto-create from EPC is intentionally retained.
	var assets int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.assets WHERE org_id=$1 AND external_key='E2801190A503006543E21224'`, orgID).Scan(&assets))
	require.Equal(t, 1, assets, "asset auto-create from EPC is retained")
}
