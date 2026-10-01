//go:build integration

package outbox_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

type alwaysEntitled struct{}

func (alwaysEntitled) OrgIsEntitled(ctx context.Context, orgID int) (bool, error) { return true, nil }

// rollbackDeliveryStore wraps a real *storage.Storage and forces the
// WithOrgTx transaction to roll back after fn has run — proving Enqueue's
// two inserts (audit row + river job) live in one transaction, not two.
// A real pgx transaction rollback is indistinguishable, from Enqueue's
// point of view, from a caller's own surrounding transaction failing after
// Enqueue returns — both discard everything written on that connection.
type rollbackDeliveryStore struct {
	*storage.Storage
	forceErr error
}

func (s rollbackDeliveryStore) WithOrgTx(ctx context.Context, orgID int, fn func(tx pgx.Tx) error) error {
	return s.Storage.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		if err := fn(tx); err != nil {
			return err
		}
		return s.forceErr
	})
}

func TestEnqueue_SourceRollbackLeavesNoJobAndNoAuditRow(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	runtime, err := outbox.NewRuntime(db.AppPool, fakeAdapter{}, db.Store, metrics)
	require.NoError(t, err)

	forcedErr := errors.New("simulated caller rollback")
	store := rollbackDeliveryStore{Storage: db.Store, forceErr: forcedErr}

	cmd := outbox.Command{DeliveryID: "evt-rollback:c1:email", OrgID: orgID, Channel: notificationdelivery.ChannelEmail}
	err = outbox.Enqueue(ctx, store, alwaysEntitled{}, runtime.Client, metrics, cmd)
	require.ErrorIs(t, err, forcedErr)

	_, getErr := db.Store.GetNotificationDeliveryByDeliveryID(ctx, orgID, cmd.DeliveryID)
	require.Error(t, getErr, "no audit row should exist after the enqueueing transaction rolled back")

	// The brief's original query checked `encoded_args::text LIKE
	// '%evt-rollback:c1:email%'` against river_job. Neither half of that
	// held up against the real schema:
	//
	//   - Task 1's migration (backend/migrations/000043_river_schema.up.sql)
	//     names the column `args` (jsonb), not `encoded_args` — River's own
	//     INSERT (riverpgxv5's dbsqlc-generated JobInsertFull query) writes
	//     to `args`.
	//   - DeliveryJobArgs (job.go) only ever carries OrgID and
	//     NotificationDeliveryID — the caller's DeliveryID string
	//     ("evt-rollback:c1:email") is never serialized into River's args at
	//     all, so a LIKE match against it could never succeed even for a job
	//     that *did* survive the rollback. That would have made the
	//     assertion pass vacuously regardless of whether atomicity held.
	//
	// testutil.SetupTestDBFull drops and recreates trakrf_test for this test
	// alone, so any row of this job kind in this database can only be the
	// one this test's Enqueue call attempted to insert — a plain count by
	// kind is both correct against the real schema and a true positive.
	var jobCount int
	scanErr := db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.river_job WHERE kind = $1`,
		outbox.DeliveryJobArgs{}.Kind(),
	).Scan(&jobCount)
	require.NoError(t, scanErr)
	require.Zero(t, jobCount, "no river_job row should exist after the enqueueing transaction rolled back")
}

func TestWorker_ProcessRestartResumesInFlightJob(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	adapter := fakeAdapter{result: outbox.Result{ProviderMessageID: "resend-msg-1"}}

	// First "process": enqueue, then stop before working it (simulates a
	// crash between commit and the worker claiming the job).
	runtime1, err := outbox.NewRuntime(db.AppPool, adapter, db.Store, metrics)
	require.NoError(t, err)
	err = outbox.Enqueue(ctx, db.Store, alwaysEntitled{}, runtime1.Client, metrics, outbox.Command{
		DeliveryID: "evt-restart:c1:email", OrgID: orgID, Channel: notificationdelivery.ChannelEmail,
	})
	require.NoError(t, err)
	// No Start() call on runtime1: nothing claims the job, simulating a
	// process that died immediately after the enqueueing transaction committed.

	// Second "process": a fresh runtime against the same pool/table picks
	// up the still-pending job.
	runtime2, err := outbox.NewRuntime(db.AppPool, adapter, db.Store, metrics)
	require.NoError(t, err)
	require.NoError(t, runtime2.Start(ctx))
	defer runtime2.Stop(ctx)

	require.Eventually(t, func() bool {
		d, err := db.Store.GetNotificationDeliveryByDeliveryID(ctx, orgID, "evt-restart:c1:email")
		return err == nil && d.State == notificationdelivery.StateDelivered
	}, 5*time.Second, 50*time.Millisecond, "a second process's runtime must pick up and complete the job the first process enqueued but never worked")
}

func TestWorker_ConcurrentRuntimesClaimJobExactlyOnce(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	var sendCount int32
	adapter := countingAdapter{count: &sendCount, result: outbox.Result{ProviderMessageID: "resend-msg-2"}}

	runtimeA, err := outbox.NewRuntime(db.AppPool, adapter, db.Store, metrics)
	require.NoError(t, err)
	runtimeB, err := outbox.NewRuntime(db.AppPool, adapter, db.Store, metrics)
	require.NoError(t, err)

	require.NoError(t, runtimeA.Start(ctx))
	require.NoError(t, runtimeB.Start(ctx))
	defer runtimeA.Stop(ctx)
	defer runtimeB.Stop(ctx)

	err = outbox.Enqueue(ctx, db.Store, alwaysEntitled{}, runtimeA.Client, metrics, outbox.Command{
		DeliveryID: "evt-concurrent:c1:email", OrgID: orgID, Channel: notificationdelivery.ChannelEmail,
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		d, err := db.Store.GetNotificationDeliveryByDeliveryID(ctx, orgID, "evt-concurrent:c1:email")
		return err == nil && d.State == notificationdelivery.StateDelivered
	}, 5*time.Second, 50*time.Millisecond)

	require.EqualValues(t, 1, sendCount, "two runtimes racing for one job must result in exactly one Send call, never a double-send")
}

type countingAdapter struct {
	count  *int32
	result outbox.Result
}

func (a countingAdapter) Send(ctx context.Context, cmd outbox.Command) (outbox.Result, error) {
	atomic.AddInt32(a.count, 1)
	return a.result, nil
}
