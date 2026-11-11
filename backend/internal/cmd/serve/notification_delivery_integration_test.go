//go:build integration

package serve

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/routing"
	"github.com/trakrf/platform/backend/internal/testutil"
)

type fakeEmailSender struct{}

func (fakeEmailSender) SendEmail(context.Context, email.Command) (email.Submission, error) {
	return email.Submission{ProviderMessageID: "fake"}, nil
}

func deliveryDeps(db *testutil.TestDB, reg prometheus.Registerer) notificationDeliveryDeps {
	return notificationDeliveryDeps{
		Store: db.Store, Pool: db.AppPool, Routing: routing.Config{PhoneRegion: "US"},
		Registry: reg, Log: zerolog.Nop(),
	}
}

func TestNotificationDelivery_BootsWithOneSetOfOutboxMetrics(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	reg := prometheus.NewRegistry()
	rt, _, err := newNotificationDelivery(deliveryDeps(db, reg))
	require.NoError(t, err)
	require.NoError(t, rt.Start(context.Background()))
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, rt.Stop(stopCtx))

	_, err = outbox.NewMetrics(reg)
	require.Error(t, err, "outbox metrics must already be registered, exactly once")
}

func TestNotificationDelivery_SurvivesStartContextCancel(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	rt, _, err := newNotificationDelivery(deliveryDeps(db, prometheus.NewRegistry()))
	require.NoError(t, err)
	startCtx, cancelStart := context.WithCancel(context.Background())
	require.NoError(t, rt.Start(context.WithoutCancel(startCtx)))
	cancelStart()
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, rt.Stop(stopCtx))
}

func TestNotificationDelivery_BuildsWithEmailAndNoSMS(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	d := deliveryDeps(db, prometheus.NewRegistry())
	d.Email = fakeEmailSender{}
	rt, _, err := newNotificationDelivery(d)
	require.NoError(t, err)
	require.NotNil(t, rt)
	require.NotNil(t, rt.Client)
}

func TestNotificationDelivery_SecondBuildOnSameRegistryFails(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	reg := prometheus.NewRegistry()
	_, _, err := newNotificationDelivery(deliveryDeps(db, reg))
	require.NoError(t, err)
	_, _, err = newNotificationDelivery(deliveryDeps(db, reg))
	require.Error(t, err, "a second build on the same registry must fail")
}

func TestNotificationDelivery_RuntimeRunsTriggerWorker(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	rt, inserter, err := newNotificationDelivery(deliveryDeps(db, prometheus.NewRegistry()))
	require.NoError(t, err)
	require.NoError(t, rt.Start(context.Background()))
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = rt.Stop(stopCtx)
	})

	// A real alarm with no subscribers: the planner loads it and queues nothing.
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-SERVE-RUNTIME")
	topic := "trakrf.id/serve-runtime/reads"
	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Reader serve runtime", Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
	})
	require.NoError(t, err)
	var scanPointID, alarmID int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT id FROM trakrf.scan_points WHERE org_id = $1 AND scan_device_id = $2 AND antenna_port = 1`,
		orgID, dev.ID).Scan(&scanPointID))
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.alarm_events (org_id, asset_id, scan_point_id, epc)
		VALUES ($1, $2, $3, 'E2801160600003') RETURNING id`,
		orgID, asset.ID, scanPointID).Scan(&alarmID))
	trigger := routing.Trigger{OrgID: orgID, Source: notificationevent.SourceAlarm, SourceID: alarmID}
	require.NoError(t, db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		_, err := routing.EnqueueTriggerTx(ctx, tx, inserter, trigger)
		return err
	}))

	require.Eventually(t, func() bool {
		var state string
		err := db.AdminPool.QueryRow(ctx,
			`SELECT state FROM trakrf.river_job WHERE kind = $1`, routing.TriggerJobArgs{}.Kind()).Scan(&state)
		return err == nil && state == "completed"
	}, 15*time.Second, 100*time.Millisecond, "the runtime must run the trigger job to completion")
}

func TestStopOnce_StopsAndReportsOnce(t *testing.T) {
	calls, reports := 0, 0
	boom := errors.New("boom")
	stop := stopOnce(func(context.Context) error { calls++; return boom }, func(err error) {
		require.ErrorIs(t, err, boom)
		reports++
	})
	stop(context.Background())
	stop(context.Background())
	require.Equal(t, 1, calls)
	require.Equal(t, 1, reports)
}
