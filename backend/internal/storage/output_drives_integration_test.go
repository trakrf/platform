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

	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func createOutput(t *testing.T, db *testutil.TestDB, orgID int, name string) int {
	t.Helper()
	dev, err := db.Store.CreateOutputDevice(context.Background(), orgID, outputdevice.CreateOutputDeviceRequest{
		Name: name, BaseURL: "http://192.0.2.10",
	})
	require.NoError(t, err)
	return dev.ID
}

func TestInsertAlarmEvent_ReturnsTheRowID(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	dev := registerDevice(t, db, orgID, "cs463-214")
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "alarm-asset")

	id, err := db.Store.InsertAlarmEvent(ctx, orgID, storage.AlarmEventRow{
		AssetID: asset.ID, ScanPointID: scanPointID(t, db, orgID, dev.ID), EPC: testEPC, RSSI: -50, FiredAt: time.Now(),
	})
	require.NoError(t, err)

	var epc string
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT epc FROM trakrf.alarm_events WHERE id = $1`, id).Scan(&epc))
	assert.Equal(t, testEPC, epc)
}

func TestRecordOutputDrive_ListsEachDeviceNewestFirst(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	horn := createOutput(t, db, orgID, "horn")
	strobe := createOutput(t, db, orgID, "strobe")
	dev := registerDevice(t, db, orgID, "cs463-214")
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "alarm-asset")
	alarmID, err := db.Store.InsertAlarmEvent(ctx, orgID, storage.AlarmEventRow{
		AssetID: asset.ID, ScanPointID: scanPointID(t, db, orgID, dev.ID), EPC: testEPC, RSSI: -50, FiredAt: time.Now(),
	})
	require.NoError(t, err)

	t0 := time.Now().Truncate(time.Millisecond)
	require.NoError(t, db.Store.RecordOutputDrive(ctx, orgID, storage.OutputDriveRow{
		OutputDeviceID: horn, AlarmEventID: &alarmID, On: true, Cause: storage.DriveCauseAlarm,
		Err: errors.New("device unreachable: dial tcp 192.0.2.10:80: i/o timeout"), Latency: 5003 * time.Millisecond, DrivenAt: t0,
	}))
	require.NoError(t, db.Store.RecordOutputDrive(ctx, orgID, storage.OutputDriveRow{
		OutputDeviceID: horn, On: false, Cause: storage.DriveCauseAgeOut, Latency: 12 * time.Millisecond, DrivenAt: t0.Add(time.Minute),
	}))
	require.NoError(t, db.Store.RecordOutputDrive(ctx, orgID, storage.OutputDriveRow{
		OutputDeviceID: strobe, AlarmEventID: &alarmID, On: true, Cause: storage.DriveCauseAlarm, Latency: 40 * time.Millisecond, DrivenAt: t0,
	}))

	got, err := db.Store.ListOutputDrives(ctx, orgID, horn, 10)
	require.NoError(t, err)
	require.Len(t, got, 2, "only the horn's drives")

	assert.Equal(t, "off", got[0].Edge)
	assert.Equal(t, "age_out", got[0].Cause)
	assert.Equal(t, "ok", got[0].Result)
	assert.Nil(t, got[0].Error)
	assert.Nil(t, got[0].AlarmEventID)
	assert.Equal(t, 12, got[0].LatencyMs)

	assert.Equal(t, "on", got[1].Edge)
	assert.Equal(t, "alarm", got[1].Cause)
	assert.Equal(t, "error", got[1].Result)
	require.NotNil(t, got[1].Error)
	assert.Equal(t, "device unreachable: dial tcp 192.0.2.10:80: i/o timeout", *got[1].Error)
	require.NotNil(t, got[1].AlarmEventID)
	assert.Equal(t, alarmID, *got[1].AlarmEventID)
	assert.Equal(t, 5003, got[1].LatencyMs)
	assert.True(t, got[1].DrivenAt.Equal(t0))
}

func TestRecordOutputDrive_SurvivesACancelledContext(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	horn := createOutput(t, db, orgID, "horn")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the HTTP client hung up before the drive returned
	require.NoError(t, db.Store.RecordOutputDrive(ctx, orgID, storage.OutputDriveRow{
		OutputDeviceID: horn, On: true, Cause: storage.DriveCauseTest, DrivenAt: time.Now(),
	}))

	got, err := db.Store.ListOutputDrives(context.Background(), orgID, horn, 10)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestOutputDrives_RLSIsolatesByOrg(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	var orgB int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ('Test Organization B', 'test-org-b', true) RETURNING id`).Scan(&orgB))
	horn := createOutput(t, db, orgA, "horn")
	require.NoError(t, db.Store.RecordOutputDrive(ctx, orgA, storage.OutputDriveRow{
		OutputDeviceID: horn, On: true, Cause: storage.DriveCauseTest, DrivenAt: time.Now(),
	}))

	var visibleToB int
	require.NoError(t, db.Store.WithOrgTx(ctx, orgB, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM trakrf.output_drives`).Scan(&visibleToB)
	}))
	assert.Equal(t, 0, visibleToB)
	got, err := db.Store.ListOutputDrives(ctx, orgA, horn, 10)
	require.NoError(t, err)
	assert.Len(t, got, 1)
}

func TestOutputDrives_OKRowCannotCarryErrorText(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	horn := createOutput(t, db, orgID, "horn")

	_, err := db.AdminPool.Exec(ctx, `
		INSERT INTO trakrf.output_drives (org_id, output_device_id, edge, cause, result, error, latency_ms, driven_at)
		VALUES ($1, $2, 'on', 'test', 'ok', 'boom', 1, now())`, orgID, horn)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output_drives_error_iff_failed")
}

// A broker-only drive that published cleanly is "sent": handed off, never
// answered. It must not read as "ok".
func TestRecordOutputDrive_UnconfirmedSuccessIsSent(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	horn := createOutput(t, db, orgID, "horn")

	require.NoError(t, db.Store.RecordOutputDrive(ctx, orgID, storage.OutputDriveRow{
		OutputDeviceID: horn, On: true, Cause: storage.DriveCauseTest, Unconfirmed: true, DrivenAt: time.Now(),
	}))
	require.NoError(t, db.Store.RecordOutputDrive(ctx, orgID, storage.OutputDriveRow{
		OutputDeviceID: horn, On: false, Cause: storage.DriveCauseTest, Unconfirmed: true,
		Err: errors.New("broker unreachable"), DrivenAt: time.Now().Add(time.Second),
	}))

	got, err := db.Store.ListOutputDrives(ctx, orgID, horn, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "error", got[0].Result, "a failed publish is still an error")
	assert.Equal(t, "sent", got[1].Result)
	assert.Nil(t, got[1].Error)
}
