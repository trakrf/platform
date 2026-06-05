//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestOutputDevice_CRUD(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	// Create with defaults (type -> shelly_gen4, switch_id -> 0, is_active -> true).
	created, err := db.Store.CreateOutputDevice(ctx, orgID, outputdevice.CreateOutputDeviceRequest{
		Name: "Demo Strobe", BaseURL: "http://192.168.50.66",
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, outputdevice.TypeShellyGen4, created.Type)
	require.Equal(t, 0, created.SwitchID)
	require.True(t, created.IsActive)
	require.Nil(t, created.LocationID)

	// Get round-trips.
	got, err := db.Store.GetOutputDeviceByID(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "Demo Strobe", got.Name)
	require.Equal(t, "http://192.168.50.66", got.BaseURL)

	// Missing id -> (nil, nil).
	missing, err := db.Store.GetOutputDeviceByID(ctx, orgID, 99999999)
	require.NoError(t, err)
	require.Nil(t, missing)

	// List + Count.
	list, err := db.Store.ListOutputDevices(ctx, orgID, 50, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	count, err := db.Store.CountOutputDevices(ctx, orgID)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	// Partial update: name, base_url, switch_id, is_active.
	newName := "Renamed Strobe"
	newURL := "http://192.168.50.99"
	newSwitch := 1
	inactive := false
	updated, err := db.Store.UpdateOutputDevice(ctx, orgID, created.ID, outputdevice.UpdateOutputDeviceRequest{
		Name: &newName, BaseURL: &newURL, SwitchID: &newSwitch, IsActive: &inactive,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, "Renamed Strobe", updated.Name)
	require.Equal(t, "http://192.168.50.99", updated.BaseURL)
	require.Equal(t, 1, updated.SwitchID)
	require.False(t, updated.IsActive)

	// Update missing id -> (nil, nil).
	missingUpd, err := db.Store.UpdateOutputDevice(ctx, orgID, 99999999, outputdevice.UpdateOutputDeviceRequest{Name: &newName})
	require.NoError(t, err)
	require.Nil(t, missingUpd)

	// Soft delete removes it from List.
	ok, err := db.Store.DeleteOutputDevice(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.True(t, ok)
	list, err = db.Store.ListOutputDevices(ctx, orgID, 50, 0)
	require.NoError(t, err)
	require.Empty(t, list)

	// Deleting again -> false.
	ok, err = db.Store.DeleteOutputDevice(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestOutputDevice_ListForLocation(t *testing.T) {
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
	bound, err := db.Store.CreateOutputDevice(ctx, orgID, outputdevice.CreateOutputDeviceRequest{
		Name: "Bound", BaseURL: "http://192.168.50.66", LocationID: &loc.ID, IsActive: &active,
	})
	require.NoError(t, err)

	// Inactive device bound to the same location -> excluded.
	inactive := false
	_, err = db.Store.CreateOutputDevice(ctx, orgID, outputdevice.CreateOutputDeviceRequest{
		Name: "Inactive", BaseURL: "http://192.168.50.67", LocationID: &loc.ID, IsActive: &inactive,
	})
	require.NoError(t, err)

	// Unbound device -> excluded.
	_, err = db.Store.CreateOutputDevice(ctx, orgID, outputdevice.CreateOutputDeviceRequest{
		Name: "Unbound", BaseURL: "http://192.168.50.68",
	})
	require.NoError(t, err)

	got, err := db.Store.ListOutputDevicesForLocation(ctx, orgID, loc.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, bound.ID, got[0].ID)

	// A location with no active bound devices -> empty.
	none, err := db.Store.ListOutputDevicesForLocation(ctx, orgID, other.ID)
	require.NoError(t, err)
	require.Empty(t, none)
}

func TestOutputDevice_OrgIsolation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	orgB := createOrg(t, db.AdminPool, "Org B", "org-b")

	created, err := db.Store.CreateOutputDevice(ctx, orgA, outputdevice.CreateOutputDeviceRequest{
		Name: "A's device", BaseURL: "http://192.168.50.66",
	})
	require.NoError(t, err)

	// Org B cannot see or fetch org A's device.
	list, err := db.Store.ListOutputDevices(ctx, orgB, 50, 0)
	require.NoError(t, err)
	require.Empty(t, list)

	got, err := db.Store.GetOutputDeviceByID(ctx, orgB, created.ID)
	require.NoError(t, err)
	require.Nil(t, got)
}
