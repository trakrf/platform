//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestGetAlarmOccurrence_ReturnsAlarmFields(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "occ-alarm")
	alarmID := f.insertAlarm(t, testInstant)

	occ, found, err := db.Store.GetAlarmOccurrence(context.Background(), orgID, alarmID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, notificationevent.SourceAlarm, occ.Source)
	require.Equal(t, alarmID, occ.SourceID)
	require.Equal(t, f.assetID, occ.AssetID)
	require.NotNil(t, occ.LocationID)
	require.Equal(t, f.locationID, *occ.LocationID)
	require.True(t, occ.OccurredAt.Equal(testInstant), "got %v", occ.OccurredAt)
}

func TestGetAlarmOccurrence_NilLocation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "occ-noloc")
	var alarmID int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.alarm_events (org_id, asset_id, scan_point_id, epc, fired_at)
		VALUES ($1, $2, $3, 'E2801160600002', $4) RETURNING id`,
		orgID, f.assetID, f.scanPointID, testInstant,
	).Scan(&alarmID))

	occ, found, err := db.Store.GetAlarmOccurrence(context.Background(), orgID, alarmID)
	require.NoError(t, err)
	require.True(t, found)
	require.Nil(t, occ.LocationID)
}

func TestGetLocationChangeOccurrence_ReturnsMoveFields(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "occ-move")
	changeID := testutil.CreateTestLocationChange(t, db, orgID, f.assetID, f.locationID, "manual_save", nil, testInstant)

	occ, found, err := db.Store.GetLocationChangeOccurrence(context.Background(), orgID, changeID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, notificationevent.SourceLocationChange, occ.Source)
	require.Equal(t, changeID, occ.SourceID)
	require.Equal(t, f.assetID, occ.AssetID)
	require.NotNil(t, occ.LocationID)
	require.Equal(t, f.locationID, *occ.LocationID)
	require.True(t, occ.OccurredAt.Equal(testInstant), "got %v", occ.OccurredAt)
}

// Another org's id and a missing id both read as "not found", never as data.
func TestOccurrence_OtherOrgAndMissingIDNotFound(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	otherOrg := createOrg(t, db.AdminPool, "Occurrence Other Org", "occurrence-other-org")
	f := newEventFixture(t, db, orgID, "occ-scope")
	alarmID := f.insertAlarm(t, testInstant)
	changeID := testutil.CreateTestLocationChange(t, db, orgID, f.assetID, f.locationID, "manual_save", nil, testInstant)
	ctx := context.Background()

	_, found, err := db.Store.GetAlarmOccurrence(ctx, otherOrg, alarmID)
	require.NoError(t, err)
	require.False(t, found)
	_, found, err = db.Store.GetLocationChangeOccurrence(ctx, otherOrg, changeID)
	require.NoError(t, err)
	require.False(t, found)

	_, found, err = db.Store.GetAlarmOccurrence(ctx, orgID, 999999999)
	require.NoError(t, err)
	require.False(t, found)
	_, found, err = db.Store.GetLocationChangeOccurrence(ctx, orgID, 999999999)
	require.NoError(t, err)
	require.False(t, found)
}
