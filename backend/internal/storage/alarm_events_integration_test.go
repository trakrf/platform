//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// scanPointID returns the device's auto-provisioned antenna-1 scan_point id (TRA-956).
func scanPointID(t *testing.T, db *testutil.TestDB, orgID, scanDeviceID int) int {
	t.Helper()
	var id int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(),
		`SELECT id FROM trakrf.scan_points WHERE org_id = $1 AND scan_device_id = $2 AND antenna_port = 1`,
		orgID, scanDeviceID).Scan(&id))
	return id
}

func TestInsertAlarmEvent_WritesUnderOrgContext(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerDevice(t, db, orgID, "cs463-214")
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "alarm-asset")
	spID := scanPointID(t, db, orgID, dev.ID)

	firedAt := time.Now()
	err := db.Store.InsertAlarmEvent(ctx, orgID, storage.AlarmEventRow{
		AssetID:     asset.ID,
		ScanPointID: spID,
		EPC:         testEPC,
		RSSI:        -50,
		TagScanID:   123,
		FiredAt:     firedAt,
	})
	require.NoError(t, err)

	var n int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.alarm_events WHERE org_id = $1 AND epc = $2`,
		orgID, testEPC).Scan(&n))
	assert.Equal(t, 1, n, "exactly one alarm_events row written")
}

func TestInsertAlarmEvent_RLSIsolatesByOrg(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	// Second org via direct insert (CreateTestAccount hardcodes a single identifier).
	var orgB int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ('Test Organization B', 'test-org-b', true) RETURNING id`).Scan(&orgB))
	dev := registerDevice(t, db, orgA, "cs463-214")
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgA, "alarm-asset")
	spID := scanPointID(t, db, orgA, dev.ID)

	require.NoError(t, db.Store.InsertAlarmEvent(ctx, orgA, storage.AlarmEventRow{
		AssetID: asset.ID, ScanPointID: spID, EPC: testEPC, RSSI: -50, FiredAt: time.Now(),
	}))

	// Under org B's RLS context the store pool must not see org A's alarm row.
	var visibleToB, visibleToA int
	require.NoError(t, db.Store.WithOrgTx(ctx, orgB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM trakrf.alarm_events`).Scan(&visibleToB)
	}))
	assert.Equal(t, 0, visibleToB, "RLS must hide org A's alarm from org B")

	// Sanity: org A's own context does see it (proves the row exists and RLS,
	// not absence, is what hid it from B).
	require.NoError(t, db.Store.WithOrgTx(ctx, orgA, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM trakrf.alarm_events`).Scan(&visibleToA)
	}))
	assert.Equal(t, 1, visibleToA, "org A sees its own alarm under its org context")
}
