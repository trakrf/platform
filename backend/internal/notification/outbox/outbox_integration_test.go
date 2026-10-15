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

// TestWorker_UnclaimedJobResumesAfterProcessRestart proves that a job
// enqueued but never claimed by any worker before a process dies (because
// runtime1.Start() is never called here) is picked up and completed by a
// second, independently-constructed runtime against the same pool/table.
//
// This does NOT prove true mid-job crash recovery (a worker dying while a
// job is actively in flight, requiring River's rescuer to requeue it after
// its stuck-job window elapses) — NewRuntime has no way to configure
// River's RescueStuckJobsAfter window (it defaults to 1 hour), which would
// make a real mid-job-crash test impractically slow without first adding
// that config plumbing to NewRuntime, a follow-up out of scope here.
func TestWorker_UnclaimedJobResumesAfterProcessRestart(t *testing.T) {
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
	}, 5*time.Second, 50*time.Millisecond, "a job enqueued but never claimed before a process dies must be picked up by a new process's runtime")
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

	require.EqualValues(t, 1, atomic.LoadInt32(&sendCount), "two runtimes racing for one job must result in exactly one Send call, never a double-send")
}

type countingAdapter struct {
	count  *int32
	result outbox.Result
}

func (a countingAdapter) Send(ctx context.Context, cmd outbox.Command) (outbox.Result, error) {
	atomic.AddInt32(a.count, 1)
	return a.result, nil
}

// failingCountingAdapter always reports a permanent provider failure and
// counts how many times it was called, so a test can prove a permanently
// failed job is never retried even though River attempts remain.
type failingCountingAdapter struct {
	count *int32
	err   error
}

func (a failingCountingAdapter) Send(ctx context.Context, cmd outbox.Command) (outbox.Result, error) {
	atomic.AddInt32(a.count, 1)
	return outbox.Result{}, a.err
}

func TestWorker_PermanentFailureIsNeverRetried(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	var sendCount int32
	adapter := failingCountingAdapter{count: &sendCount, err: &outbox.ProviderError{Kind: outbox.ErrorPermanent}}

	runtime, err := outbox.NewRuntime(db.AppPool, adapter, db.Store, metrics)
	require.NoError(t, err)
	require.NoError(t, runtime.Start(ctx))
	defer runtime.Stop(ctx)

	err = outbox.Enqueue(ctx, db.Store, alwaysEntitled{}, runtime.Client, metrics, outbox.Command{
		DeliveryID: "evt-permanent:c1:email", OrgID: orgID, Channel: notificationdelivery.ChannelEmail,
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		d, err := db.Store.GetNotificationDeliveryByDeliveryID(ctx, orgID, "evt-permanent:c1:email")
		return err == nil && d.State == notificationdelivery.StatePermanentlyFailed
	}, 5*time.Second, 50*time.Millisecond, "a permanent failure must be recorded as permanently_failed, not left pending or in_flight")

	// The TRA-398 ladder's first rung is 1s; give it time to pass. If the
	// river.JobCancel fix regressed and River treated this as an ordinary
	// retryable error, Send would be called again around that mark.
	time.Sleep(2 * time.Second)
	require.EqualValues(t, 1, atomic.LoadInt32(&sendCount), "a permanent failure must never be retried, no matter how many attempts remain")

	var jobState string
	scanErr := db.AdminPool.QueryRow(ctx,
		`SELECT state FROM trakrf.river_job WHERE kind = $1`,
		outbox.DeliveryJobArgs{}.Kind(),
	).Scan(&jobState)
	require.NoError(t, scanErr)
	require.Equal(t, "cancelled", jobState, "river.JobCancel must leave the job in River's own cancelled state, not retryable/available")
}

func TestWorker_AmbiguousSubmissionRetriesThroughProviderIdempotencyKey(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	// First attempt: adapter reports outcome-unknown (provider may have
	// accepted, response was lost). Second attempt: adapter succeeds,
	// simulating the provider's own idempotency key preventing a duplicate
	// send on retry.
	adapter := &sequenceAdapter{
		results: []adapterCall{
			{err: &outbox.ProviderError{Kind: outbox.ErrorOutcomeUnknown}},
			{result: outbox.Result{ProviderMessageID: "resend-msg-3"}},
		},
	}

	runtime, err := outbox.NewRuntime(db.AppPool, adapter, db.Store, metrics)
	require.NoError(t, err)
	require.NoError(t, runtime.Start(ctx))
	defer runtime.Stop(ctx)

	err = outbox.Enqueue(ctx, db.Store, alwaysEntitled{}, runtime.Client, metrics, outbox.Command{
		DeliveryID: "evt-ambiguous:c1:email", OrgID: orgID, Channel: notificationdelivery.ChannelEmail,
	})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		d, err := db.Store.GetNotificationDeliveryByDeliveryID(ctx, orgID, "evt-ambiguous:c1:email")
		return err == nil && d.State == notificationdelivery.StateDelivered
	}, 40*time.Second, 100*time.Millisecond, "an ambiguous first attempt must retry (per the TRA-398 ladder's 1s first rung) and succeed on the second, not be discarded as permanent nor silently marked delivered on the first ambiguous outcome")

	d, err := db.Store.GetNotificationDeliveryByDeliveryID(ctx, orgID, "evt-ambiguous:c1:email")
	require.NoError(t, err)
	require.Equal(t, 2, d.AttemptCount, "exactly two attempts: the ambiguous one and the retry that succeeded")

	require.Len(t, adapter.sentDeliveryIDs, 2, "both the ambiguous attempt and its retry must have reached the adapter")
	require.NotEmpty(t, adapter.sentDeliveryIDs[0], "the delivery ID that would be the provider idempotency key must not be empty")
	require.Equal(t, adapter.sentDeliveryIDs[0], adapter.sentDeliveryIDs[1], "a retry after an ambiguous outcome must carry the same delivery ID (the provider idempotency key), not a blind resend with no idempotency linkage")
}

type adapterCall struct {
	result outbox.Result
	err    error
}

// sequenceAdapter returns its configured results in order, one per Send
// call, and records the DeliveryID it was called with each time so tests can
// confirm a retry carries the same provider idempotency key rather than
// just blindly resending.
type sequenceAdapter struct {
	results         []adapterCall
	next            int
	sentDeliveryIDs []string
}

func (a *sequenceAdapter) Send(ctx context.Context, cmd outbox.Command) (outbox.Result, error) {
	a.sentDeliveryIDs = append(a.sentDeliveryIDs, cmd.DeliveryID)
	call := a.results[a.next]
	if a.next < len(a.results)-1 {
		a.next++
	}
	return call.result, call.err
}
