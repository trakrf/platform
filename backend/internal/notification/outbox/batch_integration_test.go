//go:build integration

package outbox_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// batchFixture is one org with a real asset and recipient (the delivery
// row's foreign keys), a River client that is never started, and a fresh
// metrics registry.
type batchFixture struct {
	db          *testutil.TestDB
	orgID       int
	assetID     int
	recipientID int
	runtime     *outbox.Runtime
	metrics     *outbox.Metrics
	registry    *prometheus.Registry
}

func newBatchFixture(t *testing.T) batchFixture {
	t.Helper()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	asset := testutil.CreateTestAsset(t, db.AdminPool, orgID, "batch-asset")

	email := "batch@acme.test"
	recipient, err := db.Store.CreateNotificationRecipient(context.Background(), orgID,
		notificationrecipient.CreateRecipientRequest{Name: "Batch", Email: &email})
	require.NoError(t, err)

	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	runtime, err := outbox.NewRuntime(db.AppPool, fakeAdapter{}, db.Store, metrics)
	require.NoError(t, err)

	return batchFixture{
		db: db, orgID: orgID, assetID: asset.ID, recipientID: recipient.ID,
		runtime: runtime, metrics: metrics, registry: registry,
	}
}

// commands builds n distinct deliveries for one event, each carrying full
// routing context and a payload unique to it.
func (f batchFixture) commands(n int) []outbox.Command {
	cmds := make([]outbox.Command, 0, n)
	for i := 1; i <= n; i++ {
		cmds = append(cmds, outbox.Command{
			DeliveryID:  fmt.Sprintf("evt-batch:c%d:email", i),
			OrgID:       f.orgID,
			Channel:     notificationdelivery.ChannelEmail,
			EventID:     "evt-batch",
			RecipientID: f.recipientID,
			AssetID:     f.assetID,
			Payload:     []byte(fmt.Sprintf(`{"subject":"Asset left zone","seq":%d}`, i)),
		})
	}
	return cmds
}

// deliveryCount counts the org's outbox rows as the admin role, so RLS
// cannot hide a row that should not be there.
func (f batchFixture) deliveryCount(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, f.db.AdminPool.QueryRow(context.Background(),
		`SELECT count(*) FROM trakrf.notification_deliveries WHERE org_id = $1`, f.orgID,
	).Scan(&n))
	return n
}

// jobCount counts delivery jobs. SetupTestDBFull recreates the database per
// test, so every job of this kind belongs to the test that counts it.
func (f batchFixture) jobCount(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, f.db.AdminPool.QueryRow(context.Background(),
		`SELECT count(*) FROM trakrf.river_job WHERE kind = $1`, outbox.DeliveryJobArgs{}.Kind(),
	).Scan(&n))
	return n
}

func TestEnqueueBatch_RecordsRowsWithPayloadAndOneJobEach(t *testing.T) {
	f := newBatchFixture(t)
	ctx := context.Background()
	cmds := f.commands(3)

	results, err := outbox.NewBatchEnqueuer(f.db.Store, f.runtime.Client, f.metrics).EnqueueBatch(ctx, f.orgID, cmds)
	require.NoError(t, err)
	require.Len(t, results, 3)

	for i, cmd := range cmds {
		require.Equal(t, cmd.DeliveryID, results[i].DeliveryID, "results come back in input order")
		require.False(t, results[i].Duplicate)
		require.NotZero(t, results[i].RiverJobID)

		d, err := f.db.Store.GetNotificationDeliveryByDeliveryID(ctx, f.orgID, cmd.DeliveryID)
		require.NoError(t, err)
		require.Equal(t, results[i].NotificationDeliveryID, d.ID)
		require.NotNil(t, d.RiverJobID)
		require.Equal(t, results[i].RiverJobID, *d.RiverJobID)
		require.Equal(t, notificationdelivery.StatePending, d.State)
		require.Equal(t, "evt-batch", *d.EventID)
		require.Equal(t, f.recipientID, *d.RecipientID)
		require.Equal(t, f.assetID, *d.AssetID)
		require.JSONEq(t, string(cmd.Payload), string(d.Payload))
	}

	require.Equal(t, 3, f.deliveryCount(t))
	require.Equal(t, 3, f.jobCount(t))
	require.Equal(t, float64(3), enqueueCount(t, f.registry, "ok"))
	require.Equal(t, float64(0), enqueueCount(t, f.registry, "duplicate"))
}

func TestEnqueueBatch_ReplayIsAllDuplicateAndQueuesNothing(t *testing.T) {
	f := newBatchFixture(t)
	ctx := context.Background()
	cmds := f.commands(3)
	enqueuer := outbox.NewBatchEnqueuer(f.db.Store, f.runtime.Client, f.metrics)

	_, err := enqueuer.EnqueueBatch(ctx, f.orgID, cmds)
	require.NoError(t, err)

	replay, err := enqueuer.EnqueueBatch(ctx, f.orgID, cmds)
	require.NoError(t, err)
	require.Len(t, replay, 3)
	for i, result := range replay {
		require.Equal(t, cmds[i].DeliveryID, result.DeliveryID)
		require.True(t, result.Duplicate, "a replayed delivery must be reported as a duplicate")
		require.Zero(t, result.NotificationDeliveryID)
		require.Zero(t, result.RiverJobID)
	}

	require.Equal(t, 3, f.deliveryCount(t), "a replay must not add rows")
	require.Equal(t, 3, f.jobCount(t), "a replay must not queue jobs")
	require.Equal(t, float64(3), enqueueCount(t, f.registry, "ok"))
	require.Equal(t, float64(3), enqueueCount(t, f.registry, "duplicate"))
}

// failingNthInserter delegates to a real River client but fails insert
// number failOn, simulating River breaking partway through a batch.
type failingNthInserter struct {
	inner  outbox.JobInserter
	failOn int32
	calls  atomic.Int32
	err    error
}

func (f *failingNthInserter) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if f.calls.Add(1) == f.failOn {
		return nil, f.err
	}
	return f.inner.InsertTx(ctx, tx, args, opts)
}

func TestEnqueueBatch_FailurePartwayLeavesNothingBehind(t *testing.T) {
	f := newBatchFixture(t)
	ctx := context.Background()

	injected := errors.New("simulated river insert failure")
	jobs := &failingNthInserter{inner: f.runtime.Client, failOn: 2, err: injected}

	results, err := outbox.NewBatchEnqueuer(f.db.Store, jobs, f.metrics).EnqueueBatch(ctx, f.orgID, f.commands(3))
	require.ErrorIs(t, err, injected)
	require.Nil(t, results)
	require.EqualValues(t, 2, jobs.calls.Load(), "the batch stops at the failing insert")

	// The first command's row and job were written before the failure; the
	// rollback must take both with it.
	require.Zero(t, f.deliveryCount(t), "no delivery row may survive a failed batch")
	require.Zero(t, f.jobCount(t), "no River job may survive a failed batch")
	require.Equal(t, float64(0), enqueueCount(t, f.registry, "ok"))
	require.Equal(t, float64(0), enqueueCount(t, f.registry, "duplicate"))
}

func TestEnqueueBatch_ConcurrentIdenticalBatchesQueueEachDeliveryOnce(t *testing.T) {
	f := newBatchFixture(t)
	ctx := context.Background()
	cmds := f.commands(3)
	enqueuer := outbox.NewBatchEnqueuer(f.db.Store, f.runtime.Client, f.metrics)

	const workers = 2
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([][]outbox.BatchResult, workers)
	errs := make([]error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			<-start
			results[w], errs[w] = enqueuer.EnqueueBatch(ctx, f.orgID, cmds)
		}(w)
	}
	close(start)
	wg.Wait()

	for w, err := range errs {
		require.NoError(t, err, "worker %d", w)
	}

	// Which worker wins each delivery is a race; only the totals are fixed.
	queued, duplicate := 0, 0
	for _, batch := range results {
		require.Len(t, batch, 3)
		for _, result := range batch {
			if result.Duplicate {
				duplicate++
			} else {
				queued++
			}
		}
	}
	require.Equal(t, 3, queued, "each delivery is queued by exactly one worker")
	require.Equal(t, 3, duplicate, "the other worker sees it as a duplicate")

	require.Equal(t, 3, f.deliveryCount(t))
	require.Equal(t, 3, f.jobCount(t))
	require.Equal(t, float64(3), enqueueCount(t, f.registry, "ok"))
	require.Equal(t, float64(3), enqueueCount(t, f.registry, "duplicate"))
}

func TestEnqueueBatch_OtherOrgCannotReadDeliveries(t *testing.T) {
	f := newBatchFixture(t)
	ctx := context.Background()
	cmds := f.commands(1)

	// CreateTestAccount uses a fixed identifier, so the second org is inserted directly.
	var otherOrgID int
	require.NoError(t, f.db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Other Co', 'other-co', true) RETURNING id`,
	).Scan(&otherOrgID))

	_, err := outbox.NewBatchEnqueuer(f.db.Store, f.runtime.Client, f.metrics).EnqueueBatch(ctx, f.orgID, cmds)
	require.NoError(t, err)

	_, err = f.db.Store.GetNotificationDeliveryByDeliveryID(ctx, f.orgID, cmds[0].DeliveryID)
	require.NoError(t, err, "the owning org reads its own delivery")

	_, err = f.db.Store.GetNotificationDeliveryByDeliveryID(ctx, otherOrgID, cmds[0].DeliveryID)
	require.ErrorContains(t, err, "not found", "another org must not read this org's delivery")

	// The lookup above also filters on org_id, so on its own it would pass
	// without RLS. Count with no org predicate under each org's context: only
	// the row-level policy decides what is visible.
	visible := func(orgID int) int {
		var n int
		require.NoError(t, f.db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM trakrf.notification_deliveries`).Scan(&n)
		}))
		return n
	}
	require.Equal(t, 1, visible(f.orgID))
	require.Zero(t, visible(otherOrgID), "RLS must hide org A's deliveries from org B")
}
