//go:build integration

package routing_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/routing"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// workerRecordingAdapter notes every command it is asked to send.
type workerRecordingAdapter struct {
	mu   sync.Mutex
	cmds []outbox.Command
}

func (a *workerRecordingAdapter) Send(_ context.Context, cmd outbox.Command) (outbox.Result, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cmds = append(a.cmds, cmd)
	return outbox.Result{ProviderMessageID: "worker-test"}, nil
}

// workerFixture is one org with an asset that has an alarm and two email
// subscribers.
type workerFixture struct {
	orgID, alarmID, subscribers int
}

func newWorkerFixture(t *testing.T, db *testutil.TestDB) workerFixture {
	t.Helper()
	ctx := context.Background()
	f := workerFixture{orgID: testutil.CreateTestAccount(t, db.AdminPool), subscribers: 2}
	asset := testutil.CreateTestAsset(t, db.AdminPool, f.orgID, "ASSET-WORKER")

	topic := "trakrf.id/worker/reads"
	dev, err := db.Store.CreateScanDevice(ctx, f.orgID, scandevice.CreateScanDeviceRequest{
		Name: "Reader worker", Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
	})
	require.NoError(t, err)
	var scanPointID int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT id FROM trakrf.scan_points WHERE org_id = $1 AND scan_device_id = $2 AND antenna_port = 1`,
		f.orgID, dev.ID).Scan(&scanPointID))
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.alarm_events (org_id, asset_id, scan_point_id, epc)
		VALUES ($1, $2, $3, 'E2801160600002') RETURNING id`,
		f.orgID, asset.ID, scanPointID).Scan(&f.alarmID))

	for _, email := range []string{"worker-a@acme.test", "worker-b@acme.test"} {
		e := email
		r, err := db.Store.CreateNotificationRecipient(ctx, f.orgID, notificationrecipient.CreateRecipientRequest{Name: e, Email: &e})
		require.NoError(t, err)
		_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, f.orgID, asset.ID, r.ID, notificationrecipient.ChannelEmail)
		require.NoError(t, err)
	}
	return f
}

func TestNewInserter_InsertsTriggerJob(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	inserter, err := outbox.NewInserter(db.AppPool)
	require.NoError(t, err)

	trigger := routing.Trigger{OrgID: orgID, Source: notificationevent.SourceAlarm, SourceID: 42}
	require.NoError(t, db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		inserted, err := routing.EnqueueTriggerTx(ctx, tx, inserter, trigger)
		require.True(t, inserted)
		return err
	}))

	var n int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.river_job WHERE kind = $1`, routing.TriggerJobArgs{}.Kind()).Scan(&n))
	require.Equal(t, 1, n)
}

func TestTriggerWorker_EndToEnd(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDBFull(t)
	f := newWorkerFixture(t, db)

	registry := prometheus.NewRegistry()
	outboxMetrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)
	routingMetrics, err := routing.NewMetrics(registry)
	require.NoError(t, err)
	renderer, err := routing.NewTemplateRenderer()
	require.NoError(t, err)

	inserter, err := outbox.NewInserter(db.AppPool)
	require.NoError(t, err)
	planner, err := routing.NewDeliveryPlanner(routing.Deps{
		Store: db.Store, Jobs: inserter, Renderer: renderer,
		Metrics: routingMetrics, OutboxMetrics: outboxMetrics,
		PhoneRegion: "US", Log: zerolog.Nop(),
	})
	require.NoError(t, err)

	adapter := &workerRecordingAdapter{}
	runtime, err := outbox.NewRuntime(db.AppPool, adapter, db.Store, outboxMetrics, routing.RegisterTriggerWorker(planner))
	require.NoError(t, err)

	trigger := routing.Trigger{OrgID: f.orgID, Source: notificationevent.SourceAlarm, SourceID: f.alarmID}
	require.NoError(t, db.Store.WithOrgTx(ctx, f.orgID, func(tx pgx.Tx) error {
		_, err := routing.EnqueueTriggerTx(ctx, tx, inserter, trigger)
		return err
	}))

	require.NoError(t, runtime.Start(ctx))
	t.Cleanup(func() { _ = runtime.Stop(context.Background()) })

	count := func(table string) int {
		var n int
		require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.`+table+` WHERE org_id = $1`, f.orgID).Scan(&n))
		return n
	}
	require.Eventually(t, func() bool {
		return count("notification_events") == 1 && count("notification_deliveries") == f.subscribers
	}, 10*time.Second, 100*time.Millisecond)

	require.NoError(t, runtime.Stop(ctx))
}
