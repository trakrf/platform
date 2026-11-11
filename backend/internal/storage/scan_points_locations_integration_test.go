//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/models/scanpoint"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestScanDeviceLocationIDs(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	locA := seedLocation(t, db.AdminPool, orgID, "SPL-A", "Inside")
	locB := seedLocation(t, db.AdminPool, orgID, "SPL-B", "Outside")
	locGone := seedLocation(t, db.AdminPool, orgID, "SPL-C", "Removed")

	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Gate", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)
	other, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Other", Type: scandevice.DeviceTypeCS463,
	})
	require.NoError(t, err)

	add := func(deviceID, port int, location *int) int {
		pt, err := db.Store.CreateScanPoint(ctx, orgID, deviceID, scanpoint.CreateScanPointRequest{
			Name: "ant", AntennaPort: &port, LocationID: location,
		})
		require.NoError(t, err)
		return pt.ID
	}
	// Antenna 1 is auto-created without a location.
	add(dev.ID, 2, &locA)
	add(dev.ID, 3, &locA) // a second antenna on the same location
	add(dev.ID, 4, &locB)
	add(dev.ID, 5, nil)
	gone := add(dev.ID, 6, &locGone)
	add(other.ID, 2, &locGone) // another device's location stays out
	ok, err := db.Store.DeleteScanPoint(ctx, orgID, gone)
	require.NoError(t, err)
	require.True(t, ok)

	var got []int
	require.NoError(t, db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		got, err = db.Store.ScanDeviceLocationIDsTx(ctx, tx, orgID, dev.ID)
		return err
	}))
	require.ElementsMatch(t, []int{locA, locB}, got)
}
