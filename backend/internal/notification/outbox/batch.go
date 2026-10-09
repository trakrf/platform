package outbox

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
)

// BatchResult reports what happened to one command of a batch, in input
// order. A Duplicate was already in the outbox: it has no new row and no job.
type BatchResult struct {
	DeliveryID             string
	NotificationDeliveryID int64 // 0 when Duplicate
	RiverJobID             int64 // 0 when Duplicate
	Duplicate              bool
}

// JobInserter is the River surface the batch path needs. *river.Client[pgx.Tx]
// satisfies it; it is an interface so tests can fail the Nth insert.
type JobInserter interface {
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

var _ JobInserter = (*river.Client[pgx.Tx])(nil)

// batchTxStore is the storage surface EnqueueBatchTx needs inside the
// caller's transaction. *storage.Storage satisfies it structurally.
type batchTxStore interface {
	InsertNotificationDeliveryIfAbsentTx(ctx context.Context, tx pgx.Tx, orgID int, d notificationdelivery.NotificationDelivery) (int64, bool, error)
	SetNotificationDeliveryRiverJobIDTx(ctx context.Context, tx pgx.Tx, orgID int, id, riverJobID int64) error
}

// batchStore adds the transaction boundary BatchEnqueuer opens itself.
type batchStore interface {
	batchTxStore
	WithOrgTx(ctx context.Context, orgID int, fn func(tx pgx.Tx) error) error
}

// EnqueueBatchTx records each command as a pending delivery and inserts its
// River job, inside the caller's transaction. It never commits or rolls
// back: any error means the caller must roll back, so the batch lands whole
// or not at all. A command whose DeliveryID is already in the outbox
// (including earlier in the same batch) is reported as Duplicate and gets
// no job. Records no metrics — the caller knows when the tx commits.
func EnqueueBatchTx(ctx context.Context, tx pgx.Tx, store batchTxStore, jobs JobInserter, cmds []Command) ([]BatchResult, error) {
	results := make([]BatchResult, 0, len(cmds))
	for _, cmd := range cmds {
		result, err := enqueueOneTx(ctx, tx, store, jobs, cmd)
		if err != nil {
			// DeliveryID only: the payload carries PII and stays out of errors.
			return nil, fmt.Errorf("failed to enqueue delivery %s: %w", cmd.DeliveryID, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func enqueueOneTx(ctx context.Context, tx pgx.Tx, store batchTxStore, jobs JobInserter, cmd Command) (BatchResult, error) {
	result := BatchResult{DeliveryID: cmd.DeliveryID}

	deliveryDBID, inserted, err := store.InsertNotificationDeliveryIfAbsentTx(ctx, tx, cmd.OrgID, toDelivery(cmd))
	if err != nil {
		return result, err
	}
	if !inserted {
		result.Duplicate = true
		return result, nil
	}

	args := DeliveryJobArgs{OrgID: cmd.OrgID, NotificationDeliveryID: deliveryDBID}
	job, err := jobs.InsertTx(ctx, tx, args, &river.InsertOpts{MaxAttempts: deliveryMaxAttempts})
	if err != nil {
		return result, fmt.Errorf("failed to enqueue river job: %w", err)
	}

	if err := store.SetNotificationDeliveryRiverJobIDTx(ctx, tx, cmd.OrgID, deliveryDBID, job.Job.ID); err != nil {
		return result, err
	}

	result.NotificationDeliveryID = deliveryDBID
	result.RiverJobID = job.Job.ID
	return result, nil
}

// toDelivery maps a command onto a pending audit row; zero routing values
// become NULL.
func toDelivery(cmd Command) notificationdelivery.NotificationDelivery {
	d := notificationdelivery.NotificationDelivery{
		DeliveryID: cmd.DeliveryID,
		Channel:    cmd.Channel,
		State:      notificationdelivery.StatePending,
		Payload:    cmd.Payload,
	}
	if cmd.EventID != "" {
		d.EventID = &cmd.EventID
	}
	if cmd.RecipientID != 0 {
		d.RecipientID = &cmd.RecipientID
	}
	if cmd.AssetID != 0 {
		d.AssetID = &cmd.AssetID
	}
	return d
}

// BatchEnqueuer enqueues a batch for one org in its own transaction and
// records metrics only once that transaction has committed.
type BatchEnqueuer struct {
	store   batchStore
	jobs    JobInserter
	metrics *Metrics
}

func NewBatchEnqueuer(store batchStore, jobs JobInserter, metrics *Metrics) *BatchEnqueuer {
	return &BatchEnqueuer{store: store, jobs: jobs, metrics: metrics}
}

// EnqueueBatch enqueues cmds atomically for orgID. An empty batch opens no
// transaction. Every command must belong to orgID; a mismatch is refused
// before any transaction opens. On error nothing is committed and nothing
// is counted.
func (e *BatchEnqueuer) EnqueueBatch(ctx context.Context, orgID int, cmds []Command) ([]BatchResult, error) {
	if len(cmds) == 0 {
		return nil, nil
	}
	for _, cmd := range cmds {
		if cmd.OrgID != orgID {
			return nil, fmt.Errorf("delivery %s belongs to org %d, not batch org %d", cmd.DeliveryID, cmd.OrgID, orgID)
		}
	}

	var results []BatchResult
	err := e.store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		results, err = EnqueueBatchTx(ctx, tx, e.store, e.jobs, cmds)
		return err
	})
	if err != nil {
		return nil, err
	}

	for _, result := range results {
		if result.Duplicate {
			e.metrics.RecordEnqueue(EnqueueDuplicate)
		} else {
			e.metrics.RecordEnqueue(EnqueueOK)
		}
	}
	return results, nil
}
