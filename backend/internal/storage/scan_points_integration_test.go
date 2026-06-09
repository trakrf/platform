//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanpoint"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestScanPoint_CRUD(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Reader", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)

	// The device auto-creates antenna 1; add a second antenna.
	port := 2
	created, err := db.Store.CreateScanPoint(ctx, orgID, dev.ID, scanpoint.CreateScanPointRequest{
		Name: "Antenna 2", AntennaPort: &port,
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, dev.ID, created.ScanDeviceID)
	require.Nil(t, created.LocationID)

	got, err := db.Store.GetScanPointByID(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Antenna 2", got.Name)

	list, err := db.Store.ListScanPointsByDevice(ctx, orgID, dev.ID)
	require.NoError(t, err)
	require.Len(t, list, 2, "auto-created antenna 1 + the added antenna 2")

	// Update a field.
	newName := "Dock Antenna"
	updated, err := db.Store.UpdateScanPoint(ctx, orgID, created.ID, scanpoint.UpdateScanPointRequest{
		Name: &newName,
	})
	require.NoError(t, err)
	require.Equal(t, "Dock Antenna", updated.Name)

	ok, err := db.Store.DeleteScanPoint(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.True(t, ok)
	list, err = db.Store.ListScanPointsByDevice(ctx, orgID, dev.ID)
	require.NoError(t, err)
	require.Len(t, list, 1, "auto-created antenna 1 remains")
}

func TestScanDevice_AutoCreatesAntenna1(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Reader", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)

	// Every device gets scan_point 1 for free (TRA-899 invariant), keyed on
	// antenna_port = 1 (TRA-956).
	pts, err := db.Store.ListScanPointsByDevice(ctx, orgID, dev.ID)
	require.NoError(t, err)
	require.Len(t, pts, 1)
	require.NotNil(t, pts[0].AntennaPort)
	require.Equal(t, 1, *pts[0].AntennaPort)
	require.Nil(t, pts[0].LocationID)
}

func TestScanPoint_DeviceDeleteCascades(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "R", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)
	// device auto-creates antenna 1; add a second so we prove BOTH cascade.
	port2 := 2
	_, err = db.Store.CreateScanPoint(ctx, orgID, dev.ID, scanpoint.CreateScanPointRequest{
		Name: "A2", AntennaPort: &port2,
	})
	require.NoError(t, err)

	ok, err := db.Store.DeleteScanDevice(ctx, orgID, dev.ID)
	require.NoError(t, err)
	require.True(t, ok)

	// Device delete soft-deletes its points too.
	pts, err := db.Store.ListScanPointsByDevice(ctx, orgID, dev.ID)
	require.NoError(t, err)
	require.Empty(t, pts)
}

func TestScanPoint_ClearLocation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	// A location to attach as the zone.
	var locID int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, 'zone-1', 'Zone 1') RETURNING id`, orgID).Scan(&locID))

	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "R", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)
	port2 := 2
	pt, err := db.Store.CreateScanPoint(ctx, orgID, dev.ID, scanpoint.CreateScanPointRequest{
		Name: "A2", AntennaPort: &port2, LocationID: &locID,
	})
	require.NoError(t, err)
	require.NotNil(t, pt.LocationID)
	require.Equal(t, locID, *pt.LocationID)

	updated, err := db.Store.UpdateScanPoint(ctx, orgID, pt.ID, scanpoint.UpdateScanPointRequest{
		ClearLocationID: true,
	})
	require.NoError(t, err)
	require.Nil(t, updated.LocationID, "ClearLocationID detaches the zone")
}
