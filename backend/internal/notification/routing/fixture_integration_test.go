//go:build integration

package routing_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/models/scandevice"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/routing"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// idleAdapter satisfies the runtime's adapter slot; nothing is ever sent
// because the River client in these tests is never started.
type idleAdapter struct{}

func (idleAdapter) Send(context.Context, outbox.Command) (outbox.Result, error) {
	return outbox.Result{}, nil
}

// plannerFixture is one org with one asset at one location, plus helpers to
// add alarms, moves and subscribers and to count what planning wrote.
type plannerFixture struct {
	db         *testutil.TestDB
	orgID      int
	assetID    int
	locationID int
	scanPoint  int
	seq        int
}

func newPlannerFixture(t *testing.T, db *testutil.TestDB, orgID int, key string) *plannerFixture {
	t.Helper()
	ctx := context.Background()
	f := &plannerFixture{db: db, orgID: orgID}
	f.assetID = testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-"+key).ID

	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.locations (org_id, external_key, name) VALUES ($1, $2, $3) RETURNING id`,
		orgID, "LOC-"+key, "Dock "+key).Scan(&f.locationID))

	topic := "trakrf.id/" + key + "/reads"
	dev, err := db.Store.CreateScanDevice(ctx, orgID, scandevice.CreateScanDeviceRequest{
		Name: "Reader " + key, Type: scandevice.DeviceTypeCS463, PublishTopic: &topic,
	})
	require.NoError(t, err)
	require.NoError(t, db.AdminPool.QueryRow(ctx, `
		SELECT id FROM trakrf.scan_points WHERE org_id = $1 AND scan_device_id = $2 AND antenna_port = 1`,
		orgID, dev.ID).Scan(&f.scanPoint))
	return f
}

func newPlannerDB(t *testing.T) (*testutil.TestDB, *plannerFixture) {
	t.Helper()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	return db, newPlannerFixture(t, db, orgID, "A")
}

// alarm inserts an alarm at the fixture's location with no notification
// event, and returns its id.
func (f *plannerFixture) alarm(t *testing.T) int {
	t.Helper()
	return f.alarmAt(t, &f.locationID)
}

// alarmAt inserts an alarm at the given location, or at none.
func (f *plannerFixture) alarmAt(t *testing.T, locationID *int) int {
	t.Helper()
	var id int
	require.NoError(t, f.db.AdminPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.alarm_events (org_id, asset_id, scan_point_id, location_id, epc)
		VALUES ($1, $2, $3, $4, 'E2801160600002') RETURNING id`,
		f.orgID, f.assetID, f.scanPoint, locationID).Scan(&id))
	return id
}

// move inserts a location change for the asset into the fixture's location.
func (f *plannerFixture) move(t *testing.T) int {
	t.Helper()
	f.seq++
	at := time.Now().UTC().Add(time.Duration(f.seq) * time.Minute).Truncate(time.Microsecond)
	return testutil.CreateTestLocationChange(t, f.db, f.orgID, f.assetID, f.locationID, "manual_save", nil, at)
}

// subscribe adds a recipient with the given contact details (empty means
// none) and subscribes it to the asset on channel. It returns the recipient id.
func (f *plannerFixture) subscribe(t *testing.T, channel, email, phone string) int {
	t.Helper()
	ctx := context.Background()
	req := notificationrecipient.CreateRecipientRequest{Name: fmt.Sprintf("Person %d", f.seq)}
	f.seq++
	if email != "" {
		req.Email = &email
	}
	if phone != "" {
		req.Phone = &phone
	}
	r, err := f.db.Store.CreateNotificationRecipient(ctx, f.orgID, req)
	require.NoError(t, err)
	_, _, err = f.db.Store.CreateAssetNotificationSubscription(ctx, f.orgID, f.assetID, r.ID, channel)
	require.NoError(t, err)
	return r.ID
}

// subscribeEmails adds n email subscribers.
func (f *plannerFixture) subscribeEmails(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		f.subscribe(t, notificationrecipient.ChannelEmail, fmt.Sprintf("person%d-%d@acme.test", i, f.seq), "")
	}
}

// planner builds a DeliveryPlanner on the real store and the given inserter.
func (f *plannerFixture) planner(t *testing.T, jobs outbox.JobInserter) *routing.DeliveryPlanner {
	t.Helper()
	renderer, err := routing.NewTemplateRenderer()
	require.NoError(t, err)
	metrics, err := routing.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	outboxMetrics, err := outbox.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	p, err := routing.NewDeliveryPlanner(routing.Deps{
		Store: f.db.Store, Jobs: jobs, Renderer: renderer,
		Metrics: metrics, OutboxMetrics: outboxMetrics, PhoneRegion: "US",
	})
	require.NoError(t, err)
	return p
}

// realJobs returns an unstarted River client: it inserts jobs but never runs
// them.
func (f *plannerFixture) realJobs(t *testing.T) *river.Client[pgx.Tx] {
	t.Helper()
	metrics, err := outbox.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	rt, err := outbox.NewRuntime(f.db.AppPool, idleAdapter{}, f.db.Store, metrics)
	require.NoError(t, err)
	return rt.Client
}

func (f *plannerFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, f.db.AdminPool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

// events counts this org's notification events from one source.
func (f *plannerFixture) events(t *testing.T, source notificationevent.Source) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM trakrf.notification_events WHERE org_id = $1 AND source = $2`,
		f.orgID, string(source))
}

// deliveries counts this org's notification deliveries.
func (f *plannerFixture) deliveries(t *testing.T) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM trakrf.notification_deliveries WHERE org_id = $1`, f.orgID)
}

// jobs counts River jobs of one kind.
func (f *plannerFixture) jobs(t *testing.T, kind string) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM trakrf.river_job WHERE kind = $1`, kind)
}

// assertNothingWritten checks the three tables are empty for this org.
func (f *plannerFixture) assertNothingWritten(t *testing.T) {
	t.Helper()
	require.Zero(t, f.events(t, notificationevent.SourceAlarm)+f.events(t, notificationevent.SourceLocationChange), "events")
	require.Zero(t, f.deliveries(t), "deliveries")
	require.Zero(t, f.jobs(t, outbox.DeliveryJobArgs{}.Kind()), "delivery jobs")
}

// failingJobs delegates to inner but fails the failOn-th InsertTx.
type failingJobs struct {
	inner  outbox.JobInserter
	failOn int
	calls  int
}

func (f *failingJobs) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	f.calls++
	if f.calls == f.failOn {
		return nil, fmt.Errorf("injected insert failure")
	}
	return f.inner.InsertTx(ctx, tx, args, opts)
}
