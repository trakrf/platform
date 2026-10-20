//go:build integration

package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/testutil"
)

const (
	sqlStateNotNull    = "23502"
	sqlStateForeignKey = "23503"
	sqlStateUnique     = "23505"
	sqlStateCheck      = "23514"
)

// requireSQLState asserts err is a Postgres error with the given SQLSTATE, so a
// test proves which constraint fired rather than that something failed.
func requireSQLState(t *testing.T, err error, code, msg string) {
	t.Helper()
	require.Error(t, err, msg)
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "%s: want a Postgres error, got %v", msg, err)
	require.Equal(t, code, pgErr.Code, "%s: got %s (%s)", msg, pgErr.Code, pgErr.Message)
}

// eventFixture is one org's asset, location and reader scan point.
type eventFixture struct {
	db          *testutil.TestDB
	orgID       int
	assetID     int
	locationID  int
	scanPointID int
}

func newEventFixture(t *testing.T, db *testutil.TestDB, orgID int, key string) eventFixture {
	t.Helper()
	ctx := context.Background()
	f := eventFixture{db: db, orgID: orgID}
	f.assetID = testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-"+key).ID

	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, $2, $2) RETURNING id`,
		orgID, "loc-"+key,
	).Scan(&f.locationID))

	topic := "trakrf.id/" + key + "/reads"
	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Reader " + key, Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
	})
	require.NoError(t, err)
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT id FROM trakrf.scan_points WHERE org_id = $1 AND scan_device_id = $2 AND antenna_port = 1`,
		orgID, dev.ID,
	).Scan(&f.scanPointID))
	return f
}

// insertAlarm writes a gate alarm for the fixture's asset at firedAt.
func (f eventFixture) insertAlarm(t *testing.T, firedAt time.Time) int {
	t.Helper()
	var id int
	require.NoError(t, f.db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.alarm_events (org_id, asset_id, scan_point_id, location_id, epc, fired_at)
		VALUES ($1, $2, $3, $4, 'E2801160600002', $5) RETURNING id`,
		f.orgID, f.assetID, f.scanPointID, f.locationID, firedAt,
	).Scan(&id))
	return id
}

// insertLocationChange writes a location change and returns the raw error, so
// constraint tests can assert on it.
func (f eventFixture) insertLocationChange(from *int, to *int, origin string, scanPointID *int, at time.Time) (int, error) {
	var id int
	err := f.db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.location_changes
		  (org_id, asset_id, from_location_id, to_location_id, origin, scan_point_id, detected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		f.orgID, f.assetID, from, to, origin, scanPointID, at,
	).Scan(&id)
	return id, err
}

// insertEvent writes a notification event and returns the raw error.
func insertEvent(db *testutil.TestDB, orgID int, source *string, assetID int, occurredAt time.Time, alarmID, changeID *int) (int, error) {
	var id int
	err := db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.notification_events
		  (org_id, source, asset_id, occurred_at, alarm_event_id, location_change_id)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		orgID, source, assetID, occurredAt, alarmID, changeID,
	).Scan(&id)
	return id, err
}

// testInstant has microsecond precision, which Postgres keeps exactly, so the
// composite foreign keys compare it equal on both sides.
var testInstant = time.Date(2026, 10, 18, 12, 0, 30, 123456000, time.UTC)

func TestNotificationEvent_AlarmSourceRoundTrips(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NE-ALARM")

	eventID := testutil.CreateTestNotificationEvent(t, db, orgID, a.ID, "ne-alarm")

	var source string
	var hasAlarm, hasChange bool
	require.NoError(t, db.AdminPool.QueryRow(context.Background(), `
		SELECT source, alarm_event_id IS NOT NULL, location_change_id IS NOT NULL
		FROM trakrf.notification_events WHERE id = $1`, eventID,
	).Scan(&source, &hasAlarm, &hasChange))
	require.Equal(t, "alarm", source)
	require.True(t, hasAlarm, "an alarm event links its alarm")
	require.False(t, hasChange, "an alarm event has no location change")
}

func TestNotificationEvent_LocationChangeSourceRoundTrips(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ne-move")

	eventID := testutil.CreateTestLocationChangeNotificationEvent(t, db, orgID, f.assetID, f.locationID, testInstant)

	var source, origin string
	var hasAlarm bool
	var toLocation int
	var occurredAt time.Time
	require.NoError(t, db.AdminPool.QueryRow(context.Background(), `
		SELECT ne.source, ne.alarm_event_id IS NOT NULL, ne.occurred_at, lc.origin, lc.to_location_id
		FROM trakrf.notification_events ne
		JOIN trakrf.location_changes lc ON lc.id = ne.location_change_id
		WHERE ne.id = $1`, eventID,
	).Scan(&source, &hasAlarm, &occurredAt, &origin, &toLocation))
	require.Equal(t, "location_change", source)
	require.False(t, hasAlarm, "a location-change event has no alarm")
	require.True(t, testInstant.Equal(occurredAt), "occurred_at is the change's detected_at")
	require.Equal(t, "manual_save", origin)
	require.Equal(t, f.locationID, toLocation)
}

// A gate read is both an alarm and a move into the gate's location, so both
// sources must be able to produce an event for the same asset at one instant.
func TestNotificationEvent_AlarmAndMoveForTheSameAssetCoexist(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ne-both")

	alarmID := f.insertAlarm(t, testInstant)
	changeID, err := f.insertLocationChange(nil, &f.locationID, "reader", &f.scanPointID, testInstant)
	require.NoError(t, err)

	_, err = insertEvent(db, orgID, strPtr("alarm"), f.assetID, testInstant, &alarmID, nil)
	require.NoError(t, err, "alarm-sourced event")
	_, err = insertEvent(db, orgID, strPtr("location_change"), f.assetID, testInstant, nil, &changeID)
	require.NoError(t, err, "location-change event for the same asset and instant")
}

func TestNotificationEvent_OneSourceIsEnforced(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ne-shape")
	alarmID := f.insertAlarm(t, testInstant)
	changeID, err := f.insertLocationChange(nil, &f.locationID, "reader", &f.scanPointID, testInstant)
	require.NoError(t, err)

	cases := []struct {
		name     string
		source   *string
		alarm    *int
		change   *int
		sqlState string
	}{
		{"alarm without its alarm", strPtr("alarm"), nil, nil, sqlStateCheck},
		{"alarm that also names a change", strPtr("alarm"), &alarmID, &changeID, sqlStateCheck},
		{"alarm that names only a change", strPtr("alarm"), nil, &changeID, sqlStateCheck},
		{"location_change without its change", strPtr("location_change"), nil, nil, sqlStateCheck},
		{"location_change that names only an alarm", strPtr("location_change"), &alarmID, nil, sqlStateCheck},
		{"location_change that also names an alarm", strPtr("location_change"), &alarmID, &changeID, sqlStateCheck},
		{"unknown source", strPtr("manual"), &alarmID, nil, sqlStateCheck},
		{"no source", nil, &alarmID, nil, sqlStateNotNull},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := insertEvent(db, orgID, tc.source, f.assetID, testInstant, tc.alarm, tc.change)
			requireSQLState(t, err, tc.sqlState, tc.name)
		})
	}
}

// The composite foreign keys make an event's asset and time equal its
// trigger's, so the copies on notification_events can never drift.
func TestNotificationEvent_MustMatchItsTrigger(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ne-match")
	otherAsset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NE-MATCH-OTHER")
	alarmID := f.insertAlarm(t, testInstant)
	changeID, err := f.insertLocationChange(nil, &f.locationID, "reader", &f.scanPointID, testInstant)
	require.NoError(t, err)
	later := testInstant.Add(time.Second)

	_, err = insertEvent(db, orgID, strPtr("alarm"), otherAsset.ID, testInstant, &alarmID, nil)
	requireSQLState(t, err, sqlStateForeignKey, "alarm event naming a different asset")
	_, err = insertEvent(db, orgID, strPtr("alarm"), f.assetID, later, &alarmID, nil)
	requireSQLState(t, err, sqlStateForeignKey, "alarm event at a different time")
	_, err = insertEvent(db, orgID, strPtr("location_change"), otherAsset.ID, testInstant, nil, &changeID)
	requireSQLState(t, err, sqlStateForeignKey, "location-change event naming a different asset")
	_, err = insertEvent(db, orgID, strPtr("location_change"), f.assetID, later, nil, &changeID)
	requireSQLState(t, err, sqlStateForeignKey, "location-change event at a different time")
}

func TestNotificationEvent_RejectsAnotherOrgsTrigger(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	otherOrgID := createOrg(t, db.AdminPool, "Notification Event Org B", "notification-event-org-b")
	other := newEventFixture(t, db, otherOrgID, "ne-xorg")
	otherAlarm := other.insertAlarm(t, testInstant)
	otherChange, err := other.insertLocationChange(nil, &other.locationID, "reader", &other.scanPointID, testInstant)
	require.NoError(t, err)

	_, err = insertEvent(db, orgID, strPtr("alarm"), other.assetID, testInstant, &otherAlarm, nil)
	requireSQLState(t, err, sqlStateForeignKey, "event pointing at another org's alarm")
	_, err = insertEvent(db, orgID, strPtr("location_change"), other.assetID, testInstant, nil, &otherChange)
	requireSQLState(t, err, sqlStateForeignKey, "event pointing at another org's location change")
}

func TestNotificationEvent_OneEventPerTrigger(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ne-once")
	alarmID := f.insertAlarm(t, testInstant)
	changeID, err := f.insertLocationChange(nil, &f.locationID, "reader", &f.scanPointID, testInstant)
	require.NoError(t, err)

	_, err = insertEvent(db, orgID, strPtr("alarm"), f.assetID, testInstant, &alarmID, nil)
	require.NoError(t, err)
	_, err = insertEvent(db, orgID, strPtr("alarm"), f.assetID, testInstant, &alarmID, nil)
	requireSQLState(t, err, sqlStateUnique, "second event for the same alarm")

	_, err = insertEvent(db, orgID, strPtr("location_change"), f.assetID, testInstant, nil, &changeID)
	require.NoError(t, err)
	_, err = insertEvent(db, orgID, strPtr("location_change"), f.assetID, testInstant, nil, &changeID)
	requireSQLState(t, err, sqlStateUnique, "second event for the same location change")
}

func TestNotificationEvent_TriggerCannotBeDeletedUnderAnEvent(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	f := newEventFixture(t, db, orgID, "ne-keep")
	alarmID := f.insertAlarm(t, testInstant)
	changeID, err := f.insertLocationChange(nil, &f.locationID, "reader", &f.scanPointID, testInstant)
	require.NoError(t, err)
	_, err = insertEvent(db, orgID, strPtr("alarm"), f.assetID, testInstant, &alarmID, nil)
	require.NoError(t, err)
	_, err = insertEvent(db, orgID, strPtr("location_change"), f.assetID, testInstant, nil, &changeID)
	require.NoError(t, err)

	_, err = db.AdminPool.Exec(ctx, `DELETE FROM trakrf.alarm_events WHERE id = $1`, alarmID)
	requireSQLState(t, err, sqlStateForeignKey, "deleting an alarm an event points at")
	_, err = db.AdminPool.Exec(ctx, `DELETE FROM trakrf.location_changes WHERE id = $1`, changeID)
	requireSQLState(t, err, sqlStateForeignKey, "deleting a location change an event points at")
}

// Short enum-like values are VARCHAR(50) enforced by the database, not TEXT
// trusted to Go; event_type is gone because it only repeated source.
func TestNotificationEvent_ColumnTypes(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()

	type col struct {
		table, column string
	}
	for _, c := range []col{{"notification_events", "source"}, {"location_changes", "origin"}} {
		var dataType string
		var maxLen *int
		require.NoError(t, db.AdminPool.QueryRow(ctx, `
			SELECT data_type, character_maximum_length FROM information_schema.columns
			WHERE table_schema = 'trakrf' AND table_name = $1 AND column_name = $2`,
			c.table, c.column,
		).Scan(&dataType, &maxLen), "%s.%s exists", c.table, c.column)
		require.Equal(t, "character varying", dataType, "%s.%s", c.table, c.column)
		require.NotNil(t, maxLen)
		require.Equal(t, 50, *maxLen, "%s.%s", c.table, c.column)
	}

	var n int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_schema = 'trakrf' AND table_name = 'notification_events' AND column_name = 'event_type'`,
	).Scan(&n))
	require.Zero(t, n, "notification_events.event_type is dropped")
}
