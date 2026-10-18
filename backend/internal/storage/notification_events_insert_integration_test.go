//go:build integration

package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// insertChange runs InsertLocationChangeIfAbsentTx in its own org transaction.
func insertChange(t *testing.T, db *testutil.TestDB, orgID int, c notificationevent.LocationChange) (int, bool, error) {
	t.Helper()
	var id int
	var inserted bool
	err := db.Store.WithOrgTx(context.Background(), orgID, func(tx pgx.Tx) error {
		var err error
		id, inserted, err = db.Store.InsertLocationChangeIfAbsentTx(context.Background(), tx, orgID, c)
		return err
	})
	return id, inserted, err
}

// insertNotificationEvent runs InsertNotificationEventIfAbsentTx in its own org transaction.
func insertNotificationEvent(t *testing.T, db *testutil.TestDB, orgID int, e notificationevent.NotificationEvent) (int, bool, error) {
	t.Helper()
	var id int
	var inserted bool
	err := db.Store.WithOrgTx(context.Background(), orgID, func(tx pgx.Tx) error {
		var err error
		id, inserted, err = db.Store.InsertNotificationEventIfAbsentTx(context.Background(), tx, orgID, e)
		return err
	})
	return id, inserted, err
}

func countRows(t *testing.T, db *testutil.TestDB, table string) int {
	t.Helper()
	var n int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(), `SELECT count(*) FROM trakrf.`+table).Scan(&n))
	return n
}

func TestInsertLocationChange_EachOriginRoundTrips(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ilc-origins")
	var yard int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(),
		`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, 'ilc-yard', 'Yard') RETURNING id`,
		orgID,
	).Scan(&yard))

	cases := []notificationevent.LocationChange{
		{Origin: notificationevent.OriginReader, ScanPointID: &f.scanPointID},
		{Origin: notificationevent.OriginManualSave, FromLocationID: &yard},
		{Origin: notificationevent.OriginManualOverride, FromLocationID: &yard},
	}
	for i, c := range cases {
		t.Run(string(c.Origin), func(t *testing.T) {
			c.OrgID, c.AssetID, c.ToLocationID = orgID, f.assetID, f.locationID
			c.DetectedAt = testInstant.Add(time.Duration(i) * time.Minute)

			id, inserted, err := insertChange(t, db, orgID, c)
			require.NoError(t, err)
			require.True(t, inserted)
			require.Greater(t, id, 0)

			var got notificationevent.LocationChange
			var origin string
			require.NoError(t, db.AdminPool.QueryRow(context.Background(), `
				SELECT org_id, asset_id, from_location_id, to_location_id, origin, scan_point_id, detected_at
				FROM trakrf.location_changes WHERE id = $1`, id,
			).Scan(&got.OrgID, &got.AssetID, &got.FromLocationID, &got.ToLocationID, &origin, &got.ScanPointID, &got.DetectedAt))
			require.Equal(t, c.OrgID, got.OrgID)
			require.Equal(t, c.AssetID, got.AssetID)
			require.Equal(t, c.FromLocationID, got.FromLocationID)
			require.Equal(t, c.ToLocationID, got.ToLocationID)
			require.Equal(t, string(c.Origin), origin)
			require.Equal(t, c.ScanPointID, got.ScanPointID)
			require.True(t, c.DetectedAt.Equal(got.DetectedAt), "detected_at round-trips exactly")
		})
	}
}

func TestInsertLocationChange_ReplayIsNotInserted(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ilc-replay")
	c := notificationevent.LocationChange{
		OrgID: orgID, AssetID: f.assetID, ToLocationID: f.locationID,
		Origin: notificationevent.OriginManualSave, DetectedAt: testInstant,
	}

	_, inserted, err := insertChange(t, db, orgID, c)
	require.NoError(t, err)
	require.True(t, inserted)

	id, inserted, err := insertChange(t, db, orgID, c)
	require.NoError(t, err, "a replay is not an error")
	require.False(t, inserted)
	require.Zero(t, id)
	require.Equal(t, 1, countRows(t, db, "location_changes"))
}

func TestInsertLocationChange_InvalidNeverReachesTheDB(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ilc-invalid")
	base := notificationevent.LocationChange{
		OrgID: orgID, AssetID: f.assetID, ToLocationID: f.locationID, DetectedAt: testInstant,
	}

	readerWithoutScanPoint := base
	readerWithoutScanPoint.Origin = notificationevent.OriginReader
	_, _, err := insertChange(t, db, orgID, readerWithoutScanPoint)
	require.True(t, errors.Is(err, notificationevent.ErrInvalid), "got %v", err)

	otherOrg := base
	otherOrg.Origin = notificationevent.OriginManualSave
	otherOrg.OrgID = orgID + 1
	_, _, err = insertChange(t, db, orgID, otherOrg)
	require.True(t, errors.Is(err, notificationevent.ErrInvalid), "a change for another org than the transaction's: got %v", err)

	require.Zero(t, countRows(t, db, "location_changes"))
}

// The insert joins the caller's transaction: a rolled-back caller leaves no row.
func TestInsertLocationChange_RollsBackWithTheCaller(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ilc-rollback")
	errAbort := errors.New("caller aborts")

	err := db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		_, inserted, err := db.Store.InsertLocationChangeIfAbsentTx(ctx, tx, orgID, notificationevent.LocationChange{
			OrgID: orgID, AssetID: f.assetID, ToLocationID: f.locationID,
			Origin: notificationevent.OriginManualSave, DetectedAt: testInstant,
		})
		require.NoError(t, err)
		require.True(t, inserted)
		return errAbort
	})
	require.ErrorIs(t, err, errAbort)
	require.Zero(t, countRows(t, db, "location_changes"))
}

func TestInsertNotificationEvent_FromAnAlarm(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ine-alarm")
	alarmID := f.insertAlarm(t, testInstant)
	e := notificationevent.NotificationEvent{
		OrgID: orgID, Source: notificationevent.SourceAlarm, AssetID: f.assetID,
		OccurredAt: testInstant, AlarmEventID: &alarmID,
	}

	id, inserted, err := insertNotificationEvent(t, db, orgID, e)
	require.NoError(t, err)
	require.True(t, inserted)
	require.Greater(t, id, 0)

	var source string
	var gotAlarm int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(),
		`SELECT source, alarm_event_id FROM trakrf.notification_events WHERE id = $1`, id,
	).Scan(&source, &gotAlarm))
	require.Equal(t, "alarm", source)
	require.Equal(t, alarmID, gotAlarm)

	id, inserted, err = insertNotificationEvent(t, db, orgID, e)
	require.NoError(t, err, "a replay is not an error")
	require.False(t, inserted)
	require.Zero(t, id)
}

func TestInsertNotificationEvent_FromALocationChange(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ine-move")
	c := notificationevent.LocationChange{
		OrgID: orgID, AssetID: f.assetID, ToLocationID: f.locationID,
		Origin: notificationevent.OriginReader, ScanPointID: &f.scanPointID, DetectedAt: testInstant,
	}

	// The detection and its event commit together, as a detector would write them.
	var eventID int
	require.NoError(t, db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		changeID, inserted, err := db.Store.InsertLocationChangeIfAbsentTx(ctx, tx, orgID, c)
		require.NoError(t, err)
		require.True(t, inserted)
		eventID, inserted, err = db.Store.InsertNotificationEventIfAbsentTx(ctx, tx, orgID, notificationevent.NotificationEvent{
			OrgID: orgID, Source: notificationevent.SourceLocationChange, AssetID: c.AssetID,
			OccurredAt: c.DetectedAt, LocationChangeID: &changeID,
		})
		require.True(t, inserted)
		return err
	}))
	require.Greater(t, eventID, 0)

	var source string
	var hasAlarm bool
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT source, alarm_event_id IS NOT NULL FROM trakrf.notification_events WHERE id = $1`, eventID,
	).Scan(&source, &hasAlarm))
	require.Equal(t, "location_change", source)
	require.False(t, hasAlarm)
}

func TestInsertNotificationEvent_InvalidNeverReachesTheDB(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ine-invalid")
	alarmID := f.insertAlarm(t, testInstant)
	changeID := testutil.CreateTestLocationChange(t, db, orgID, f.assetID, f.locationID, "manual_save", nil, testInstant)

	bothTriggers := notificationevent.NotificationEvent{
		OrgID: orgID, Source: notificationevent.SourceAlarm, AssetID: f.assetID,
		OccurredAt: testInstant, AlarmEventID: &alarmID, LocationChangeID: &changeID,
	}
	_, _, err := insertNotificationEvent(t, db, orgID, bothTriggers)
	require.True(t, errors.Is(err, notificationevent.ErrInvalid), "got %v", err)

	otherOrg := bothTriggers
	otherOrg.LocationChangeID = nil
	otherOrg.OrgID = orgID + 1
	_, _, err = insertNotificationEvent(t, db, orgID, otherOrg)
	require.True(t, errors.Is(err, notificationevent.ErrInvalid), "an event for another org than the transaction's: got %v", err)

	require.Zero(t, countRows(t, db, "notification_events"))
}

// ON CONFLICT DO NOTHING absorbs only duplicates. An event that does not
// match its trigger is a real error, never reported as "already enqueued".
func TestInsertNotificationEvent_MismatchedTriggerIsAnError(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ine-mismatch")
	alarmID := f.insertAlarm(t, testInstant)

	_, inserted, err := insertNotificationEvent(t, db, orgID, notificationevent.NotificationEvent{
		OrgID: orgID, Source: notificationevent.SourceAlarm, AssetID: f.assetID,
		OccurredAt: testInstant.Add(time.Second), AlarmEventID: &alarmID,
	})
	requireSQLState(t, err, sqlStateForeignKey, "event time differs from its alarm's")
	require.False(t, inserted)
	require.False(t, errors.Is(err, notificationevent.ErrInvalid), "a database rejection is not ErrInvalid")
	require.Zero(t, countRows(t, db, "notification_events"))
}
