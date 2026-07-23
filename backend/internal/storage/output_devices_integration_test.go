//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/outputdevice"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
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

func TestOutputDevice_ClearLocation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	loc, err := db.Store.CreateLocation(ctx, location.Location{
		OrgID: orgID, ExternalKey: "dock-1", Name: "Dock 1",
	})
	require.NoError(t, err)

	dev, err := db.Store.CreateOutputDevice(ctx, orgID, outputdevice.CreateOutputDeviceRequest{
		Name: "Bound Strobe", BaseURL: "http://192.168.50.66", LocationID: &loc.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, dev.LocationID)
	require.Equal(t, loc.ID, *dev.LocationID)

	// An update that omits location_id leaves it attached.
	newName := "Renamed"
	kept, err := db.Store.UpdateOutputDevice(ctx, orgID, dev.ID, outputdevice.UpdateOutputDeviceRequest{
		Name: &newName,
	})
	require.NoError(t, err)
	require.NotNil(t, kept.LocationID, "omitting location_id leaves the binding unchanged")
	require.Equal(t, loc.ID, *kept.LocationID)

	// ClearLocationID detaches the location (sets the column NULL).
	cleared, err := db.Store.UpdateOutputDevice(ctx, orgID, dev.ID, outputdevice.UpdateOutputDeviceRequest{
		ClearLocationID: true,
	})
	require.NoError(t, err)
	require.Nil(t, cleared.LocationID, "ClearLocationID detaches the location")
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

// TestOutputDevice_ReaderBaseTopic covers TRA-1028: a GPO output device
// addressed by scan_device_id resolves ReaderBaseTopic from the reader's
// publish_topic (minus the /reads suffix) on both the fire read
// (GetOutputDeviceByID) and the geofence-firer read
// (ListOutputDevicesForLocation). Also covers ScanDeviceExistsInOrg.
func TestOutputDevice_ReaderBaseTopic(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	publishTopic := "trakrf.id/cs463-212/reads"
	reader, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "CS463-212", Type: "csl_cs463", PublishTopic: &publishTopic,
	})
	require.NoError(t, err)

	loc, err := db.Store.CreateLocation(ctx, location.Location{
		OrgID: orgID, ExternalKey: "dock-gpo", Name: "GPO Dock",
	})
	require.NoError(t, err)

	created, err := db.Store.CreateOutputDevice(ctx, orgID, outputdevice.CreateOutputDeviceRequest{
		Name: "Reader GPO", Type: outputdevice.TypeCS463GPO, Transport: outputdevice.TransportMQTT,
		ScanDeviceID: &reader.ID, LocationID: &loc.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, created.ScanDeviceID)
	require.Equal(t, reader.ID, *created.ScanDeviceID)
	// CreateOutputDevice's RETURNING path has no join: ReaderBaseTopic is
	// transient and populated only by the two read paths under test below.
	require.Empty(t, created.ReaderBaseTopic)

	// GetOutputDeviceByID (test-fire path) resolves ReaderBaseTopic.
	got, err := db.Store.GetOutputDeviceByID(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "trakrf.id/cs463-212", got.ReaderBaseTopic)

	// ListOutputDevicesForLocation (fire path) resolves it too.
	list, err := db.Store.ListOutputDevicesForLocation(ctx, orgID, loc.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "trakrf.id/cs463-212", list[0].ReaderBaseTopic)

	// A device with no scan_device_id: ReaderBaseTopic stays empty on both reads.
	plain, err := db.Store.CreateOutputDevice(ctx, orgID, outputdevice.CreateOutputDeviceRequest{
		Name: "Plain Shelly", BaseURL: "http://192.168.50.70",
	})
	require.NoError(t, err)
	gotPlain, err := db.Store.GetOutputDeviceByID(ctx, orgID, plain.ID)
	require.NoError(t, err)
	require.Empty(t, gotPlain.ReaderBaseTopic)
}

func TestOutputDevice_ReaderBaseTopic_CrossOrgReaderStaysEmpty(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	orgB := createOrg(t, db.AdminPool, "Org B Reader", "org-b-reader")

	publishTopic := "trakrf.id/cs463-999/reads"
	readerB, err := db.Store.CreateScanDevice(ctx, orgB, scandevice.CreateScanDeviceRequest{
		Name: "Org B Reader", Type: "csl_cs463", PublishTopic: &publishTopic,
	})
	require.NoError(t, err)

	// An output device in org A that somehow references org B's scan device id
	// (e.g. a stale row) must not resolve ReaderBaseTopic: the JOIN runs under
	// org A's RLS GUC, so readerB is invisible and the join simply misses.
	created, err := db.Store.CreateOutputDevice(ctx, orgA, outputdevice.CreateOutputDeviceRequest{
		Name: "Cross-org GPO", Type: outputdevice.TypeCS463GPO, Transport: outputdevice.TransportMQTT,
		ScanDeviceID: &readerB.ID,
	})
	require.NoError(t, err)

	got, err := db.Store.GetOutputDeviceByID(ctx, orgA, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got.ReaderBaseTopic, "cross-org reader must not resolve under RLS")
}

// TestCheckGPOReader covers the storage-level GPO reader validation added to
// close the two write-time gaps found in final review (TRA-1028): reader type
// must be csl_cs463, and it must have a non-empty publish_topic. Found/
// cross-org behavior mirrors the former ScanDeviceExistsInOrg this replaces.
func TestCheckGPOReader(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	orgB := createOrg(t, db.AdminPool, "Org B Exists", "org-b-exists")

	publishTopic := "trakrf.id/cs463-555/reads"
	reader, err := db.Store.CreateScanDevice(ctx, orgA, scandevice.CreateScanDeviceRequest{
		Name: "CS463-555", Type: "csl_cs463", PublishTopic: &publishTopic,
	})
	require.NoError(t, err)

	// Happy path: found, right type, has a publish_topic.
	check, err := db.Store.CheckGPOReader(ctx, orgA, reader.ID)
	require.NoError(t, err)
	require.True(t, check.Found)
	require.True(t, check.IsCS463)
	require.True(t, check.HasPublishTopic)

	// Missing id: not found.
	missing, err := db.Store.CheckGPOReader(ctx, orgA, 99999999)
	require.NoError(t, err)
	require.False(t, missing.Found)

	// In-org id from another org's perspective: RLS makes it read as absent.
	crossOrg, err := db.Store.CheckGPOReader(ctx, orgB, reader.ID)
	require.NoError(t, err)
	require.False(t, crossOrg.Found)

	// A reader of the wrong type: found, but not a CS463.
	glReader, err := db.Store.CreateScanDevice(ctx, orgA, scandevice.CreateScanDeviceRequest{
		Name: "GL-S10", Type: "gl_s10",
	})
	require.NoError(t, err)
	glCheck, err := db.Store.CheckGPOReader(ctx, orgA, glReader.ID)
	require.NoError(t, err)
	require.True(t, glCheck.Found)
	require.False(t, glCheck.IsCS463)

	// A csl_cs463 reader with no publish_topic: found, right type, no topic.
	noTopicReader, err := db.Store.CreateScanDevice(ctx, orgA, scandevice.CreateScanDeviceRequest{
		Name: "CS463-No-Topic", Type: "csl_cs463",
	})
	require.NoError(t, err)
	noTopicCheck, err := db.Store.CheckGPOReader(ctx, orgA, noTopicReader.ID)
	require.NoError(t, err)
	require.True(t, noTopicCheck.Found)
	require.True(t, noTopicCheck.IsCS463)
	require.False(t, noTopicCheck.HasPublishTopic)
}
