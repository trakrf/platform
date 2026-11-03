//go:build integration

package dispatch_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/notification/dispatch"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/routing"
	"github.com/trakrf/platform/backend/internal/testutil"
)

var _ outbox.ErrorReasoner = (*dispatch.Error)(nil)

const e2eAddress = "e2e@acme.test"

// e2eEmail records every command and returns a fixed result.
type e2eEmail struct {
	mu   sync.Mutex
	cmds []email.Command
	sub  email.Submission
	err  error
}

func (f *e2eEmail) SendEmail(_ context.Context, c email.Command) (email.Submission, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, c)
	return f.sub, f.err
}

func (f *e2eEmail) sent() []email.Command {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]email.Command(nil), f.cmds...)
}

type e2eFixture struct {
	db       *testutil.TestDB
	orgID    int
	trigger  routing.Trigger
	planner  *routing.DeliveryPlanner
	inserter *river.Client[pgx.Tx]
	runtime  *outbox.Runtime
	adapter  *dispatch.Adapter
	metrics  *outbox.Metrics
	sender   *e2eEmail
}

// newE2EFixture builds one org with an alarm and a single email subscriber,
// wired to a real runtime and the dispatch adapter. The runtime is not started.
func newE2EFixture(t *testing.T, sender *e2eEmail) *e2eFixture {
	t.Helper()
	ctx := context.Background()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-E2E")

	topic := "trakrf.id/e2e/reads"
	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Reader e2e", Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
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

	addr := e2eAddress
	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: addr, Email: &addr})
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, asset.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)

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
	adapter, err := dispatch.New(dispatch.Deps{Email: sender, Suppressions: db.Store, Log: zerolog.Nop()})
	require.NoError(t, err)
	runtime, err := outbox.NewRuntime(db.AppPool, adapter, db.Store, outboxMetrics, routing.RegisterTriggerWorker(planner))
	require.NoError(t, err)

	return &e2eFixture{
		db: db, orgID: orgID, planner: planner, inserter: inserter, runtime: runtime,
		adapter: adapter, metrics: outboxMetrics, sender: sender,
		trigger: routing.Trigger{OrgID: orgID, Source: notificationevent.SourceAlarm, SourceID: alarmID},
	}
}

func (f *e2eFixture) start(t *testing.T) {
	t.Helper()
	require.NoError(t, f.runtime.Start(context.Background()))
	t.Cleanup(func() { _ = f.runtime.Stop(context.Background()) })
}

// enqueueTrigger puts the trigger job on the queue; the started runtime plans it.
func (f *e2eFixture) enqueueTrigger(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, f.db.Store.WithOrgTx(ctx, f.orgID, func(tx pgx.Tx) error {
		_, err := routing.EnqueueTriggerTx(ctx, tx, f.inserter, f.trigger)
		return err
	}))
}

type deliveryRow struct {
	ID         int64
	State      string
	Attempts   int
	ErrorKind  *string
	ProviderID *string
	RiverJobID *int64
}

// delivery reads the org's single delivery row; ok is false until it exists.
// Any error other than "no row yet" fails the test.
func (f *e2eFixture) delivery(t *testing.T) (deliveryRow, bool) {
	t.Helper()
	var d deliveryRow
	err := f.db.AdminPool.QueryRow(context.Background(), `
		SELECT id, state, attempt_count, last_error_kind, provider_message_id, river_job_id
		FROM trakrf.notification_deliveries WHERE org_id = $1`, f.orgID).
		Scan(&d.ID, &d.State, &d.Attempts, &d.ErrorKind, &d.ProviderID, &d.RiverJobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return deliveryRow{}, false
	}
	require.NoError(t, err)
	return d, true
}

// poll runs check every 100ms for up to 10s. check returns whether it is done
// and a description of what it last saw, which the failure message reports.
func poll(t *testing.T, what string, check func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		ok, seen := check()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			require.FailNowf(t, "timed out", "%s; last seen: %s", what, seen)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (f *e2eFixture) waitDelivery(t *testing.T, done func(deliveryRow) bool) deliveryRow {
	t.Helper()
	var got deliveryRow
	poll(t, "delivery never reached the wanted state", func() (bool, string) {
		d, ok := f.delivery(t)
		got = d
		if !ok {
			return false, "no delivery row"
		}
		kind := "<nil>"
		if d.ErrorKind != nil {
			kind = *d.ErrorKind
		}
		return done(d), fmt.Sprintf("state=%q attempts=%d error_kind=%s", d.State, d.Attempts, kind)
	})
	return got
}

// jobState reads a river job's state; ok is false if the job row is absent.
func (f *e2eFixture) jobState(t *testing.T, jobID int64) (string, bool) {
	t.Helper()
	var s string
	err := f.db.AdminPool.QueryRow(context.Background(),
		`SELECT state::text FROM trakrf.river_job WHERE id = $1`, jobID).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false
	}
	require.NoError(t, err)
	return s, true
}

func (f *e2eFixture) jobStateEventually(t *testing.T, jobID int64, want string) {
	t.Helper()
	poll(t, fmt.Sprintf("river job %d never reached %q", jobID, want), func() (bool, string) {
		s, ok := f.jobState(t, jobID)
		if !ok {
			return false, "no river_job row"
		}
		return s == want, fmt.Sprintf("state=%q", s)
	})
}

func TestDispatch_EndToEnd_Delivered(t *testing.T) {
	ctx := context.Background()
	sender := &e2eEmail{sub: email.Submission{ProviderMessageID: "re_e2e"}}
	f := newE2EFixture(t, sender)
	f.enqueueTrigger(t)
	f.start(t)

	d := f.waitDelivery(t, func(d deliveryRow) bool { return d.State == "delivered" })
	require.NotNil(t, d.ProviderID)
	require.Equal(t, "re_e2e", *d.ProviderID)
	require.Nil(t, d.ErrorKind)
	require.NotNil(t, d.RiverJobID)
	f.jobStateEventually(t, *d.RiverJobID, "completed")

	cmds := sender.sent()
	require.Len(t, cmds, 1)
	require.Equal(t, e2eAddress, cmds[0].To)
	require.Empty(t, cmds[0].HTML)
	require.NotEmpty(t, cmds[0].Text)

	// The sender saw exactly the message the planner stored on the row.
	var payloadTo, payloadSubject, payloadBody string
	require.NoError(t, f.db.AdminPool.QueryRow(ctx, `
		SELECT payload->>'to', payload->>'subject', payload->>'body'
		FROM trakrf.notification_deliveries WHERE id = $1`, d.ID).
		Scan(&payloadTo, &payloadSubject, &payloadBody))
	require.Equal(t, payloadTo, cmds[0].To)
	require.NotEmpty(t, payloadSubject)
	require.Equal(t, payloadSubject, cmds[0].Subject)
	require.Equal(t, payloadBody, cmds[0].Text)
	require.NotEmpty(t, cmds[0].DeliveryID)

	// Running the same job again must not send twice: the row is already
	// delivered, so the worker skips it.
	worker := outbox.NewDeliveryWorker(f.db.Store, f.adapter, f.metrics)
	job := &river.Job[outbox.DeliveryJobArgs]{
		JobRow: &rivertype.JobRow{ID: *d.RiverJobID, Attempt: 1, MaxAttempts: 6},
		Args:   outbox.DeliveryJobArgs{OrgID: f.orgID, NotificationDeliveryID: d.ID},
	}
	require.NoError(t, worker.Work(ctx, job))
	require.Len(t, sender.sent(), 1)
	again, _ := f.delivery(t)
	require.Equal(t, "delivered", again.State)
	require.Equal(t, d.Attempts, again.Attempts)
}

func TestDispatch_EndToEnd_PermanentError(t *testing.T) {
	sender := &e2eEmail{err: &email.ProviderError{Kind: email.ErrorInvalid}}
	f := newE2EFixture(t, sender)
	f.enqueueTrigger(t)
	f.start(t)

	d := f.waitDelivery(t, func(d deliveryRow) bool { return d.State == "permanently_failed" })
	require.NotNil(t, d.ErrorKind)
	require.Equal(t, "invalid", *d.ErrorKind)
	require.Nil(t, d.ProviderID)
	require.Len(t, sender.sent(), 1)
	require.NotNil(t, d.RiverJobID)
	f.jobStateEventually(t, *d.RiverJobID, "cancelled")
}

func TestDispatch_EndToEnd_TransientErrorRetries(t *testing.T) {
	sender := &e2eEmail{err: &email.ProviderError{Kind: email.ErrorTransient}}
	f := newE2EFixture(t, sender)
	f.enqueueTrigger(t)
	f.start(t)

	d := f.waitDelivery(t, func(d deliveryRow) bool { return d.State == "in_flight" && d.Attempts >= 1 })
	require.NotNil(t, d.ErrorKind)
	require.Equal(t, "transient", *d.ErrorKind)
	require.Nil(t, d.ProviderID)
	require.NotEmpty(t, sender.sent())
	require.NotNil(t, d.RiverJobID)
	f.jobStateEventually(t, *d.RiverJobID, "retryable")
}

func TestDispatch_EndToEnd_OptedOutAfterPlanning(t *testing.T) {
	ctx := context.Background()
	sender := &e2eEmail{sub: email.Submission{ProviderMessageID: "re_never"}}
	f := newE2EFixture(t, sender)

	// Plan without the runtime running: the delivery row and its job exist,
	// but nothing has claimed the job yet.
	_, err := f.planner.Plan(ctx, f.trigger)
	require.NoError(t, err)
	planned, ok := f.delivery(t)
	require.True(t, ok)
	require.Equal(t, "pending", planned.State)
	require.NotNil(t, planned.RiverJobID)

	// The recipient opts out between planning and sending.
	_, err = f.db.AdminPool.Exec(ctx, `
		INSERT INTO trakrf.notification_suppressions (org_id, channel, address, reason)
		VALUES ($1, 'email', $2, 'manual')`, f.orgID, e2eAddress)
	require.NoError(t, err)

	f.start(t)

	d := f.waitDelivery(t, func(d deliveryRow) bool { return d.State == "permanently_failed" })
	require.NotNil(t, d.ErrorKind)
	require.Equal(t, "opted_out", *d.ErrorKind)
	require.Empty(t, sender.sent())
	f.jobStateEventually(t, *d.RiverJobID, "cancelled")
}
