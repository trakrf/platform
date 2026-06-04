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
		ExternalKey: "cs463-214", Name: "Reader", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)

	port := 1
	created, err := db.Store.CreateScanPoint(ctx, orgID, dev.ID, scanpoint.CreateScanPointRequest{
		ExternalKey: "cs463-214-1", Name: "Antenna 1", AntennaPort: &port,
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, dev.ID, created.ScanDeviceID)
	require.False(t, created.IsBoundary, "is_boundary defaults false")
	require.Nil(t, created.LocationID)

	got, err := db.Store.GetScanPointByID(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Antenna 1", got.Name)

	list, err := db.Store.ListScanPointsByDevice(ctx, orgID, dev.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)

	// Toggle boundary on.
	boundary := true
	updated, err := db.Store.UpdateScanPoint(ctx, orgID, created.ID, scanpoint.UpdateScanPointRequest{
		IsBoundary: &boundary,
	})
	require.NoError(t, err)
	require.True(t, updated.IsBoundary)

	ok, err := db.Store.DeleteScanPoint(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.True(t, ok)
	list, err = db.Store.ListScanPointsByDevice(ctx, orgID, dev.ID)
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestScanPoint_DeviceDeleteCascades(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		ExternalKey: "cs463-9", Name: "R", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)
	_, err = db.Store.CreateScanPoint(ctx, orgID, dev.ID, scanpoint.CreateScanPointRequest{
		ExternalKey: "cs463-9-1", Name: "A1",
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
		ExternalKey: "cs463-z", Name: "R", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)
	pt, err := db.Store.CreateScanPoint(ctx, orgID, dev.ID, scanpoint.CreateScanPointRequest{
		ExternalKey: "cs463-z-1", Name: "A1", LocationID: &locID,
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
