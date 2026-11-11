//go:build integration

package routing_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/notification/routing"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// noopTriggerWorker only registers the kind: River refuses to insert a job
// kind its client has no worker for, and the real worker is not built yet.
type noopTriggerWorker struct {
	river.WorkerDefaults[routing.TriggerJobArgs]
}

func (noopTriggerWorker) Work(context.Context, *river.Job[routing.TriggerJobArgs]) error {
	return nil
}

func TestEnqueueTriggerTx_SameTriggerIsOneJob(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDBFull(t)
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	// An unstarted client: it inserts jobs but never claims or runs them.
	workers := river.NewWorkers()
	river.AddWorker(workers, noopTriggerWorker{})
	client, err := river.NewClient(riverpgxv5.New(db.AppPool), &river.Config{Workers: workers})
	require.NoError(t, err)

	trigger := routing.Trigger{OrgID: orgID, Source: notificationevent.SourceAlarm, SourceID: 42}
	enqueue := func() bool {
		var inserted bool
		require.NoError(t, db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
			var err error
			inserted, err = routing.EnqueueTriggerTx(ctx, tx, client, trigger)
			return err
		}))
		return inserted
	}

	require.True(t, enqueue(), "first enqueue inserts the job")
	require.False(t, enqueue(), "same trigger is skipped as a duplicate")

	var n int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.river_job WHERE kind = $1`, routing.TriggerJobArgs{}.Kind(),
	).Scan(&n))
	require.Equal(t, 1, n)
}
