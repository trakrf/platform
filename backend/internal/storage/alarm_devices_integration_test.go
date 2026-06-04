//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/alarmdevice"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestAlarmDevice_CRUD(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	// Create with defaults (type -> shelly_gen4, switch_id -> 0, is_active -> true).
	created, err := db.Store.CreateAlarmDevice(ctx, orgID, alarmdevice.CreateAlarmDeviceRequest{
		Name: "Demo Strobe", BaseURL: "http://192.168.50.66",
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, alarmdevice.TypeShellyGen4, created.Type)
	require.Equal(t, 0, created.SwitchID)
	require.True(t, created.IsActive)
	require.Nil(t, created.LocationID)

	// Get round-trips.
	got, err := db.Store.GetAlarmDeviceByID(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "Demo Strobe", got.Name)
	require.Equal(t, "http://192.168.50.66", got.BaseURL)

	// Missing id -> (nil, nil).
	missing, err := db.Store.GetAlarmDeviceByID(ctx, orgID, 99999999)
	require.NoError(t, err)
	require.Nil(t, missing)

	// List + Count.
	list, err := db.Store.ListAlarmDevices(ctx, orgID, 50, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	count, err := db.Store.CountAlarmDevices(ctx, orgID)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	// Partial update: name, base_url, switch_id, is_active.
	newName := "Renamed Strobe"
	newURL := "http://192.168.50.99"
	newSwitch := 1
	inactive := false
	updated, err := db.Store.UpdateAlarmDevice(ctx, orgID, created.ID, alarmdevice.UpdateAlarmDeviceRequest{
		Name: &newName, BaseURL: &newURL, SwitchID: &newSwitch, IsActive: &inactive,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, "Renamed Strobe", updated.Name)
	require.Equal(t, "http://192.168.50.99", updated.BaseURL)
	require.Equal(t, 1, updated.SwitchID)
	require.False(t, updated.IsActive)

	// Update missing id -> (nil, nil).
	missingUpd, err := db.Store.UpdateAlarmDevice(ctx, orgID, 99999999, alarmdevice.UpdateAlarmDeviceRequest{Name: &newName})
	require.NoError(t, err)
	require.Nil(t, missingUpd)

	// Soft delete removes it from List.
	ok, err := db.Store.DeleteAlarmDevice(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.True(t, ok)
	list, err = db.Store.ListAlarmDevices(ctx, orgID, 50, 0)
	require.NoError(t, err)
	require.Empty(t, list)

	// Deleting again -> false.
	ok, err = db.Store.DeleteAlarmDevice(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestAlarmDevice_ListForLocation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	loc, err := db.Store.CreateLocation(ctx, location.Location{
		OrgID: orgID, ExternalKey: "dock-1", Name: "Dock 1",
	})
	require.NoError(t, err)
	other, err := db.Store.CreateLocation(ctx, location.Location{
		OrgID: orgID, ExternalKey: "dock-2", Name: "Dock 2",
	})
	require.NoError(t, err)

	// Active device bound to the location -> should be returned.
	active := true
	bound, err := db.Store.CreateAlarmDevice(ctx, orgID, alarmdevice.CreateAlarmDeviceRequest{
		Name: "Bound", BaseURL: "http://192.168.50.66", LocationID: &loc.ID, IsActive: &active,
	})
	require.NoError(t, err)

	// Inactive device bound to the same location -> excluded.
	inactive := false
	_, err = db.Store.CreateAlarmDevice(ctx, orgID, alarmdevice.CreateAlarmDeviceRequest{
		Name: "Inactive", BaseURL: "http://192.168.50.67", LocationID: &loc.ID, IsActive: &inactive,
	})
	require.NoError(t, err)

	// Unbound device -> excluded.
	_, err = db.Store.CreateAlarmDevice(ctx, orgID, alarmdevice.CreateAlarmDeviceRequest{
		Name: "Unbound", BaseURL: "http://192.168.50.68",
	})
	require.NoError(t, err)

	got, err := db.Store.ListAlarmDevicesForLocation(ctx, orgID, loc.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, bound.ID, got[0].ID)

	// A location with no active bound devices -> empty.
	none, err := db.Store.ListAlarmDevicesForLocation(ctx, orgID, other.ID)
	require.NoError(t, err)
	require.Empty(t, none)
}

func TestAlarmDevice_OrgIsolation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	orgB := createOrg(t, db.AdminPool, "Org B", "org-b")

	created, err := db.Store.CreateAlarmDevice(ctx, orgA, alarmdevice.CreateAlarmDeviceRequest{
		Name: "A's device", BaseURL: "http://192.168.50.66",
	})
	require.NoError(t, err)

	// Org B cannot see or fetch org A's device.
	list, err := db.Store.ListAlarmDevices(ctx, orgB, 50, 0)
	require.NoError(t, err)
	require.Empty(t, list)

	got, err := db.Store.GetAlarmDeviceByID(ctx, orgB, created.ID)
	require.NoError(t, err)
	require.Nil(t, got)
}
