//go:build integration

package routing_test

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/routing"
)

const deliveryKind = "notification_delivery"

func alarmTrigger(f *plannerFixture, id int) routing.Trigger {
	return routing.Trigger{OrgID: f.orgID, Source: notificationevent.SourceAlarm, SourceID: id}
}

func moveTrigger(f *plannerFixture, id int) routing.Trigger {
	return routing.Trigger{OrgID: f.orgID, Source: notificationevent.SourceLocationChange, SourceID: id}
}

// assertScheduled checks one event of source with n deliveries and n jobs,
// each delivery tied to the event, a recipient and the asset.
func assertScheduled(t *testing.T, f *plannerFixture, source notificationevent.Source, sourceColumn string, sourceID, n int) {
	t.Helper()
	ctx := context.Background()
	require.Equal(t, 1, f.events(t, source))

	var eventID int
	var gotSourceID *int
	require.NoError(t, f.db.AdminPool.QueryRow(ctx,
		`SELECT id, `+sourceColumn+` FROM trakrf.notification_events WHERE org_id = $1 AND source = $2`,
		f.orgID, string(source)).Scan(&eventID, &gotSourceID))
	require.NotNil(t, gotSourceID)
	require.Equal(t, sourceID, *gotSourceID)

	require.Equal(t, n, f.deliveries(t))
	require.Equal(t, n, f.count(t, `
		SELECT count(*) FROM trakrf.notification_deliveries
		WHERE org_id = $1 AND event_id = $2 AND recipient_id IS NOT NULL AND asset_id = $3
		  AND payload->>'source' = $4 AND river_job_id IS NOT NULL`,
		f.orgID, itoa(eventID), f.assetID, string(source)))
	require.Equal(t, n, f.jobs(t, deliveryKind))
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func TestPlan_AlarmTriggerSchedulesDeliveries(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribeEmails(t, 3)
	alarmID := f.alarm(t)

	res, err := f.planner(t, f.realJobs(t)).Plan(context.Background(), alarmTrigger(f, alarmID))
	require.NoError(t, err)
	require.Len(t, res.Recipients, 3)
	for _, r := range res.Recipients {
		require.Equal(t, routing.OutcomeEnqueued, r.Outcome)
		require.NotEmpty(t, r.DeliveryID)
	}
	assertScheduled(t, f, notificationevent.SourceAlarm, "alarm_event_id", alarmID, 3)
}

func TestPlan_LocationChangeTriggerSchedulesDeliveries(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribeEmails(t, 3)
	moveID := f.move(t)

	res, err := f.planner(t, f.realJobs(t)).Plan(context.Background(), moveTrigger(f, moveID))
	require.NoError(t, err)
	require.Len(t, res.Recipients, 3)
	assertScheduled(t, f, notificationevent.SourceLocationChange, "location_change_id", moveID, 3)
}

func TestPlan_GateReadAsBothRegistersTwoEvents(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribeEmails(t, 3)
	alarmID, moveID := f.alarm(t), f.move(t)
	p := f.planner(t, f.realJobs(t))

	_, err := p.Plan(context.Background(), alarmTrigger(f, alarmID))
	require.NoError(t, err)
	_, err = p.Plan(context.Background(), moveTrigger(f, moveID))
	require.NoError(t, err)

	require.Equal(t, 1, f.events(t, notificationevent.SourceAlarm))
	require.Equal(t, 1, f.events(t, notificationevent.SourceLocationChange))
	require.Equal(t, 6, f.deliveries(t))
	require.Equal(t, 6, f.count(t, `SELECT count(DISTINCT delivery_id) FROM trakrf.notification_deliveries WHERE org_id = $1`, f.orgID))
	require.Equal(t, 6, f.jobs(t, deliveryKind))
}

func TestPlan_TwiceAddsNothing(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribeEmails(t, 3)
	alarmID := f.alarm(t)
	p := f.planner(t, f.realJobs(t))

	_, err := p.Plan(context.Background(), alarmTrigger(f, alarmID))
	require.NoError(t, err)
	res, err := p.Plan(context.Background(), alarmTrigger(f, alarmID))
	require.NoError(t, err)

	require.Equal(t, routing.OutcomeDuplicate, res.Outcome)
	assertScheduled(t, f, notificationevent.SourceAlarm, "alarm_event_id", alarmID, 3)
}

func TestPlan_LateSubscriberGetsNothingOnReplay(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribeEmails(t, 2)
	alarmID := f.alarm(t)
	p := f.planner(t, f.realJobs(t))

	_, err := p.Plan(context.Background(), alarmTrigger(f, alarmID))
	require.NoError(t, err)
	f.subscribe(t, notificationrecipient.ChannelEmail, "late@acme.test", "")
	res, err := p.Plan(context.Background(), alarmTrigger(f, alarmID))
	require.NoError(t, err)

	require.Equal(t, routing.OutcomeDuplicate, res.Outcome)
	assertScheduled(t, f, notificationevent.SourceAlarm, "alarm_event_id", alarmID, 2)
}

func TestPlan_ConcurrentPlansScheduleOnce(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribeEmails(t, 3)
	alarmID := f.alarm(t)
	p := f.planner(t, f.realJobs(t))

	start := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = p.Plan(context.Background(), alarmTrigger(f, alarmID))
		}()
	}
	close(start)
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assertScheduled(t, f, notificationevent.SourceAlarm, "alarm_event_id", alarmID, 3)
}

func TestPlan_SoftDeletedAssetRegistersEventOnly(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribeEmails(t, 2)
	alarmID := f.alarm(t)
	_, err := f.db.AdminPool.Exec(context.Background(), `UPDATE trakrf.assets SET deleted_at = now() WHERE id = $1`, f.assetID)
	require.NoError(t, err)

	res, err := f.planner(t, f.realJobs(t)).Plan(context.Background(), alarmTrigger(f, alarmID))
	require.NoError(t, err)

	require.Equal(t, routing.OutcomeSkippedAssetDeleted, res.Outcome)
	require.Equal(t, 1, f.events(t, notificationevent.SourceAlarm))
	require.Zero(t, f.deliveries(t))
	require.Zero(t, f.jobs(t, deliveryKind))
}

func TestPlan_ZeroSubscribersRegistersEventOnly(t *testing.T) {
	_, f := newPlannerDB(t)
	alarmID := f.alarm(t)

	res, err := f.planner(t, f.realJobs(t)).Plan(context.Background(), alarmTrigger(f, alarmID))
	require.NoError(t, err)

	require.Equal(t, routing.OutcomeSkippedNoSubscribers, res.Outcome)
	require.Equal(t, 1, f.events(t, notificationevent.SourceAlarm))
	require.Zero(t, f.deliveries(t))
	require.Zero(t, f.jobs(t, deliveryKind))
}

func TestPlan_UnentitledOrgWritesNothing(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribeEmails(t, 2)
	alarmID := f.alarm(t)
	_, err := f.db.AdminPool.Exec(context.Background(),
		`UPDATE trakrf.organizations SET subscription_enabled = false WHERE id = $1`, f.orgID)
	require.NoError(t, err)

	res, err := f.planner(t, f.realJobs(t)).Plan(context.Background(), alarmTrigger(f, alarmID))
	require.NoError(t, err)

	require.Equal(t, routing.OutcomeSkippedUnentitled, res.Outcome)
	f.assertNothingWritten(t)
}

func TestPlan_FailureOnSecondJobWritesNothing(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribeEmails(t, 3)
	alarmID := f.alarm(t)
	jobs := &failingJobs{inner: f.realJobs(t), failOn: 2}

	_, err := f.planner(t, jobs).Plan(context.Background(), alarmTrigger(f, alarmID))

	require.Error(t, err)
	require.NotErrorIs(t, err, routing.ErrInvalidTrigger)
	require.GreaterOrEqual(t, jobs.calls, 2)
	f.assertNothingWritten(t)
}

func TestPlan_OtherOrgSourceIsInvalid(t *testing.T) {
	db, f := newPlannerDB(t)
	f.subscribeEmails(t, 2)

	var otherOrg int
	require.NoError(t, db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Other', 'other-org', true) RETURNING id`).Scan(&otherOrg))
	other := newPlannerFixture(t, db, otherOrg, "B")
	foreignAlarm := other.alarm(t)

	_, err := f.planner(t, f.realJobs(t)).Plan(context.Background(), alarmTrigger(f, foreignAlarm))

	require.ErrorIs(t, err, routing.ErrInvalidTrigger)
	f.assertNothingWritten(t)
	other.assertNothingWritten(t)
}

func TestPlan_SuppressionMatchesAcrossPhoneFormats(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribe(t, notificationrecipient.ChannelSMS, "", "(555) 123-4567")
	alarmID := f.alarm(t)
	_, err := f.db.AdminPool.Exec(context.Background(), `
		INSERT INTO trakrf.notification_suppressions (org_id, channel, address, reason)
		VALUES ($1, 'sms', '+15551234567', 'sms_stop')`, f.orgID)
	require.NoError(t, err)

	res, err := f.planner(t, f.realJobs(t)).Plan(context.Background(), alarmTrigger(f, alarmID))
	require.NoError(t, err)

	require.Len(t, res.Recipients, 1)
	require.Equal(t, notificationdelivery.Channel("sms"), res.Recipients[0].Channel)
	require.Equal(t, routing.OutcomeSkippedOptedOut, res.Recipients[0].Outcome)
	require.Equal(t, 1, f.events(t, notificationevent.SourceAlarm))
	require.Zero(t, f.count(t, `SELECT count(*) FROM trakrf.notification_deliveries WHERE org_id = $1 AND channel = 'sms'`, f.orgID))
	require.Zero(t, f.jobs(t, deliveryKind))
}

func TestPlan_CancelledContextWritesNothing(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribeEmails(t, 2)
	alarmID := f.alarm(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := f.planner(t, f.realJobs(t)).Plan(ctx, alarmTrigger(f, alarmID))

	require.Error(t, err)
	require.NotErrorIs(t, err, routing.ErrInvalidTrigger)
	f.assertNothingWritten(t)
}

func TestPlan_AlarmWithoutLocationSaysUnknown(t *testing.T) {
	_, f := newPlannerDB(t)
	f.subscribeEmails(t, 1)
	// A move always has a destination; an alarm may have none.
	alarmID := f.alarmAt(t, nil)

	_, err := f.planner(t, f.realJobs(t)).Plan(context.Background(), alarmTrigger(f, alarmID))
	require.NoError(t, err)

	require.Equal(t, 1, f.count(t, `
		SELECT count(*) FROM trakrf.notification_deliveries
		WHERE org_id = $1 AND payload->>'body' LIKE '%an unknown location%'`, f.orgID))
}

var _ outbox.JobInserter = (*failingJobs)(nil)
