//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

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

func TestGetLocationChangeOccurrence_LoadsOriginAndFrom(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "occ-origin")
	ctx := context.Background()

	var fromID int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, 'loc-occ-from', 'loc-occ-from') RETURNING id`,
		orgID,
	).Scan(&fromID))

	insert := func(origin string, at time.Time) int {
		var id int
		require.NoError(t, db.AdminPool.QueryRow(ctx, `
			INSERT INTO trakrf.location_changes
				(org_id, asset_id, from_location_id, to_location_id, origin, scan_point_id, detected_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			orgID, f.assetID, fromID, f.locationID, origin, f.scanPointID, at,
		).Scan(&id))
		return id
	}

	crossing, found, err := db.Store.GetLocationChangeOccurrence(ctx, orgID, insert("reader_crossing", testInstant))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, notificationevent.OriginReaderCrossing, crossing.Origin)
	require.NotNil(t, crossing.FromLocationID)
	require.Equal(t, fromID, *crossing.FromLocationID)
	require.True(t, crossing.IsCrossing())

	plain, found, err := db.Store.GetLocationChangeOccurrence(ctx, orgID, insert("reader", testInstant.Add(time.Minute)))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, notificationevent.OriginReader, plain.Origin)
	require.False(t, plain.IsCrossing())

	alarm, found, err := db.Store.GetAlarmOccurrence(ctx, orgID, f.insertAlarm(t, testInstant))
	require.NoError(t, err)
	require.True(t, found)
	require.Nil(t, alarm.FromLocationID)
	require.False(t, alarm.IsCrossing())
}
