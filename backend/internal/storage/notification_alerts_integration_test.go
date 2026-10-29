//go:build integration

package storage_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/notificationalert"
	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/testutil"
)

type alertSeed struct {
	eventID, recipientID, assetID int
	channel                       notificationdelivery.Channel
	payload                       string
	state                         notificationdelivery.State
	sentAt                        time.Time
}

// seedAlert writes one delivery row in state s.state and backdates its
// finalized_at to s.sentAt, and returns the row id.
func seedAlert(t *testing.T, db *testutil.TestDB, orgID int, s alertSeed) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	err := db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		id, _, err = db.Store.InsertNotificationDeliveryIfAbsentTx(ctx, tx, orgID, notificationdelivery.NotificationDelivery{
			DeliveryID:  fmt.Sprintf("%d|%d|%s", s.eventID, s.recipientID, s.channel),
			Channel:     s.channel,
			State:       notificationdelivery.StatePending,
			EventID:     &s.eventID,
			RecipientID: &s.recipientID,
			AssetID:     &s.assetID,
			Payload:     []byte(s.payload),
		})
		return err
	})
	require.NoError(t, err)
	if s.state != notificationdelivery.StatePending {
		require.NoError(t, db.Store.UpdateNotificationDeliveryState(ctx, orgID, id, s.state, nil, nil))
	}
	if s.state == notificationdelivery.StateDelivered || s.state == notificationdelivery.StatePermanentlyFailed {
		_, err = db.AdminPool.Exec(ctx, `UPDATE trakrf.notification_deliveries SET finalized_at = $1 WHERE id = $2`, s.sentAt, id)
		require.NoError(t, err)
	}
	return id
}

type alertFixture struct {
	orgID                    int
	assetA, assetB           int
	alice, bob               int
	alarmEmail, alarmSMS     int64
	moveEmail                int64
	alarmAt, moveAt, baseNow time.Time
}

// newAlertFixture: asset A's gate alarm at Gate A reached Alice by email and
// Bob by SMS; asset B's move from Row A to Row B reached Alice by email. One
// pending and one failed delivery must never be listed.
func newAlertFixture(t *testing.T, db *testutil.TestDB) alertFixture {
	t.Helper()
	ctx := context.Background()
	f := alertFixture{orgID: testutil.CreateTestAccount(t, db.AdminPool)}
	f.baseNow = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	f.assetA = testutil.CreateTestAsset(t, db.AdminPool, f.orgID, "ALERT-A").ID
	f.assetB = testutil.CreateTestAsset(t, db.AdminPool, f.orgID, "ALERT-B").ID

	loc := func(key, name string) int {
		var id int
		require.NoError(t, db.AdminPool.QueryRow(ctx,
			`INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, $2, $3) RETURNING id`,
			f.orgID, key, name).Scan(&id))
		return id
	}
	gate, rowA, rowB := loc("gate-a", "Gate A"), loc("row-a", "Row A"), loc("row-b", "Row B")

	recipient := func(name, email, phone string) int {
		r, err := db.Store.CreateNotificationRecipient(ctx, f.orgID, notificationrecipient.CreateRecipientRequest{
			Name: name, Email: strPtr(email), Phone: strPtr(phone),
		})
		require.NoError(t, err)
		return r.ID
	}
	f.alice = recipient("Alice", "alice@acme.test", "+15550000001")
	f.bob = recipient("Bob", "bob@acme.test", "+15550000002")

	alarmEvent := testutil.CreateTestNotificationEvent(t, db, f.orgID, f.assetA, "alerts-list")
	_, err := db.AdminPool.Exec(ctx, `UPDATE trakrf.alarm_events SET location_id = $1
		WHERE id = (SELECT alarm_event_id FROM trakrf.notification_events WHERE id = $2)`, gate, alarmEvent)
	require.NoError(t, err)
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT occurred_at FROM trakrf.notification_events WHERE id = $1`, alarmEvent).Scan(&f.alarmAt))

	f.moveAt = f.baseNow.Add(-48 * time.Hour)
	moveEvent := testutil.CreateTestLocationChangeNotificationEvent(t, db, f.orgID, f.assetB, rowB, f.moveAt)
	_, err = db.AdminPool.Exec(ctx, `UPDATE trakrf.location_changes SET from_location_id = $1
		WHERE id = (SELECT location_change_id FROM trakrf.notification_events WHERE id = $2)`, rowA, moveEvent)
	require.NoError(t, err)

	delivered := notificationdelivery.StateDelivered
	f.alarmEmail = seedAlert(t, db, f.orgID, alertSeed{alarmEvent, f.alice, f.assetA, notificationdelivery.ChannelEmail,
		`{"to":"alice@acme.test","subject":"Alert: Test Asset left the yard","body":"left through Gate A"}`,
		delivered, f.baseNow.Add(-1 * time.Hour)})
	f.alarmSMS = seedAlert(t, db, f.orgID, alertSeed{alarmEvent, f.bob, f.assetA, notificationdelivery.ChannelSMS,
		`{"to":"+15550000002","body":"left through Gate A"}`,
		delivered, f.baseNow.Add(-2 * time.Hour)})
	f.moveEmail = seedAlert(t, db, f.orgID, alertSeed{moveEvent, f.alice, f.assetB, notificationdelivery.ChannelEmail,
		`{"to":"alice@acme.test","subject":"Moved","body":"moved from Row A to Row B"}`,
		delivered, f.baseNow.Add(-47 * time.Hour)})

	seedAlert(t, db, f.orgID, alertSeed{moveEvent, f.bob, f.assetB, notificationdelivery.ChannelEmail,
		`{"to":"bob@acme.test","subject":"Moved","body":"pending"}`,
		notificationdelivery.StatePending, time.Time{}})
	seedAlert(t, db, f.orgID, alertSeed{moveEvent, f.bob, f.assetB, notificationdelivery.ChannelSMS,
		`{"to":"+15550000002","body":"failed"}`,
		notificationdelivery.StatePermanentlyFailed, f.baseNow})
	return f
}

func listAlerts(t *testing.T, db *testutil.TestDB, orgID int, f notificationalert.ListFilter) ([]notificationalert.Alert, int) {
	t.Helper()
	if f.Limit == 0 {
		f.Limit = 20
	}
	alerts, total, err := db.Store.ListNotificationAlerts(context.Background(), orgID, f)
	require.NoError(t, err)
	return alerts, total
}

func alertIDs(alerts []notificationalert.Alert) []int64 {
	out := make([]int64, 0, len(alerts))
	for _, a := range alerts {
		out = append(out, a.ID)
	}
	return out
}

func TestListNotificationAlerts_DeliveredOnlyNewestFirstWithNames(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	f := newAlertFixture(t, db)

	alerts, total := listAlerts(t, db, f.orgID, notificationalert.ListFilter{})
	require.Equal(t, 3, total)
	require.Equal(t, []int64{f.alarmEmail, f.alarmSMS, f.moveEmail}, alertIDs(alerts))

	email := alerts[0]
	require.Equal(t, f.assetA, email.AssetID)
	require.Equal(t, "Test Asset", email.AssetName)
	require.Equal(t, "ALERT-A", email.AssetExternalKey)
	require.Equal(t, notificationevent.SourceAlarm, email.Type)
	require.True(t, f.alarmAt.Equal(email.OccurredAt))
	require.True(t, f.baseNow.Add(-1*time.Hour).Equal(email.SentAt))
	require.Equal(t, f.alice, email.RecipientID)
	require.Equal(t, "Alice", email.RecipientName)
	require.Equal(t, notificationdelivery.ChannelEmail, email.Channel)
	require.Equal(t, "Alert: Test Asset left the yard", *email.Subject)
	require.Equal(t, "left through Gate A", email.Body)
	require.Equal(t, map[string]string{"location": "Gate A"}, email.Details)

	require.Nil(t, alerts[1].Subject, "SMS has no subject")
	require.Equal(t, "Bob", alerts[1].RecipientName)

	move := alerts[2]
	require.Equal(t, notificationevent.SourceLocationChange, move.Type)
	require.True(t, f.moveAt.Equal(move.OccurredAt))
	require.Equal(t, map[string]string{"from": "Row A", "to": "Row B"}, move.Details)
}

func TestListNotificationAlerts_Filters(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	f := newAlertFixture(t, db)
	alarm := notificationevent.SourceAlarm
	sms := notificationdelivery.ChannelSMS
	from := f.baseNow.Add(-3 * time.Hour)
	to := f.baseNow.Add(-90 * time.Minute)

	cases := []struct {
		name   string
		filter notificationalert.ListFilter
		want   []int64
	}{
		{"asset", notificationalert.ListFilter{AssetID: &f.assetB}, []int64{f.moveEmail}},
		{"recipient", notificationalert.ListFilter{RecipientID: &f.alice}, []int64{f.alarmEmail, f.moveEmail}},
		{"type", notificationalert.ListFilter{Type: &alarm}, []int64{f.alarmEmail, f.alarmSMS}},
		{"channel", notificationalert.ListFilter{Channel: &sms}, []int64{f.alarmSMS}},
		{"from", notificationalert.ListFilter{From: &from}, []int64{f.alarmEmail, f.alarmSMS}},
		{"from and to", notificationalert.ListFilter{From: &from, To: &to}, []int64{f.alarmSMS}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alerts, total := listAlerts(t, db, f.orgID, tc.filter)
			require.Equal(t, tc.want, alertIDs(alerts))
			require.Equal(t, len(tc.want), total)
		})
	}
}

func TestListNotificationAlerts_PagesKeepTheTotal(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	f := newAlertFixture(t, db)

	alerts, total := listAlerts(t, db, f.orgID, notificationalert.ListFilter{Limit: 2, Offset: 2})
	require.Equal(t, 3, total)
	require.Equal(t, []int64{f.moveEmail}, alertIDs(alerts))

	alerts, total = listAlerts(t, db, f.orgID, notificationalert.ListFilter{Limit: 2, Offset: 10})
	require.Equal(t, 3, total, "a page past the end still reports the total")
	require.Empty(t, alerts)
	require.NotNil(t, alerts, "an empty page encodes as [], not null")
}

func TestListNotificationAlerts_OrgIsolation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	newAlertFixture(t, db)
	otherOrgID := createOrg(t, db.AdminPool, "Alerts Org B", "alerts-org-b")

	alerts, total := listAlerts(t, db, otherOrgID, notificationalert.ListFilter{})
	require.Zero(t, total)
	require.Empty(t, alerts)
}
