//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestScanDevice_CRUD(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	// Create with transport defaulting to mqtt; publish_topic set directly (TRA-956).
	topic := "trakrf.id/cs463-214/reads"
	created, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Dock Reader", Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, scandevice.TransportMQTT, created.Transport)
	require.NotNil(t, created.PublishTopic)
	require.Equal(t, topic, *created.PublishTopic)
	require.True(t, created.IsActive)

	// Get round-trips.
	got, err := db.Store.GetScanDeviceByID(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "Dock Reader", got.Name)

	// Missing id -> (nil, nil).
	missing, err := db.Store.GetScanDeviceByID(ctx, orgID, 99999999)
	require.NoError(t, err)
	require.Nil(t, missing)

	// List + Count.
	list, err := db.Store.ListScanDevices(ctx, orgID, 50, 0)
	require.NoError(t, err)
	require.Len(t, list, 1)
	count, err := db.Store.CountScanDevices(ctx, orgID)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	// Update name + explicit publish_topic.
	newName := "Renamed Reader"
	newTopic := "trakrf.id/custom/reads"
	updated, err := db.Store.UpdateScanDevice(ctx, orgID, created.ID, scandevice.UpdateScanDeviceRequest{
		Name: &newName, PublishTopic: &newTopic,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, "Renamed Reader", updated.Name)
	require.Equal(t, newTopic, *updated.PublishTopic)

	// Soft delete removes it from List.
	ok, err := db.Store.DeleteScanDevice(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.True(t, ok)
	list, err = db.Store.ListScanDevices(ctx, orgID, 50, 0)
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestScanDevice_PublishTopicUniquePerOrg(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	topic := "trakrf.id/dup/reads"
	_, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "A", Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
	})
	require.NoError(t, err)
	_, err = db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "B", Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
	})
	require.Error(t, err, "duplicate publish_topic within an org must be rejected")
}

func TestScanDevice_OrgIsolation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)

	// Second org seeded directly (CreateTestAccount uses a fixed identifier).
	var orgB int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.organizations (name, identifier, is_active)
		VALUES ('Org B', 'test-org-b', true) RETURNING id`).Scan(&orgB))

	topicA := "trakrf.id/cs463-a/reads"
	dev, err := db.Store.CreateScanDevice(ctx, orgA, scandevice.CreateScanDeviceRequest{
		Name: "A", Type: scandevice.DeviceTypeCS463, PublishTopic: &topicA,
	})
	require.NoError(t, err)

	// Org B cannot see org A's device.
	got, err := db.Store.GetScanDeviceByID(ctx, orgB, dev.ID)
	require.NoError(t, err)
	require.Nil(t, got)
	list, err := db.Store.ListScanDevices(ctx, orgB, 50, 0)
	require.NoError(t, err)
	require.Empty(t, list)
}
