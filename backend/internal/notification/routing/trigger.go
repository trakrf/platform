// Package routing turns a detected alarm or location change into scheduled
// deliveries. It owns the trigger job a detector queues in its own
// transaction; the planner and worker that consume it build on the names here.
package routing

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
)

// ErrInvalidTrigger marks a Trigger that names no real org, source or row.
var ErrInvalidTrigger = errors.New("routing: invalid trigger")

// Trigger says which detected row to notify about: an alarm or a location
// change in one org.
type Trigger struct {
	OrgID    int
	Source   notificationevent.Source
	SourceID int
}

// Validate reports ErrInvalidTrigger unless the org and source row ids are
// positive and the source is one the planner knows.
func (t Trigger) Validate() error {
	if t.OrgID <= 0 {
		return fmt.Errorf("%w: org id %d", ErrInvalidTrigger, t.OrgID)
	}
	if t.SourceID <= 0 {
		return fmt.Errorf("%w: source id %d", ErrInvalidTrigger, t.SourceID)
	}
	switch t.Source {
	case notificationevent.SourceAlarm, notificationevent.SourceLocationChange:
		return nil
	}
	return fmt.Errorf("%w: unknown source %q", ErrInvalidTrigger, t.Source)
}

// TriggerJobArgs is what goes into river_job.args. It holds ids only, never
// names, emails or phone numbers.
type TriggerJobArgs struct {
	OrgID    int    `json:"org_id"`
	Source   string `json:"source"`
	SourceID int    `json:"source_id"`
}

func (TriggerJobArgs) Kind() string { return "notification_trigger" }

// InsertOpts makes the same trigger one job: River skips an insert whose
// args match a job it already holds.
func (TriggerJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		MaxAttempts: 6,
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// Trigger rebuilds the Trigger the job was queued for.
func (a TriggerJobArgs) Trigger() Trigger {
	return Trigger{OrgID: a.OrgID, Source: notificationevent.Source(a.Source), SourceID: a.SourceID}
}

// EnqueueTriggerTx queues the trigger job inside the caller's transaction, so
// it exists only if the detector's own write commits. inserted is false when
// an identical trigger is already queued.
func EnqueueTriggerTx(ctx context.Context, tx pgx.Tx, jobs outbox.JobInserter, t Trigger) (inserted bool, err error) {
	if err := t.Validate(); err != nil {
		return false, err
	}
	// nil opts: River takes MaxAttempts and uniqueness from TriggerJobArgs.
	res, err := jobs.InsertTx(ctx, tx, TriggerJobArgs{OrgID: t.OrgID, Source: string(t.Source), SourceID: t.SourceID}, nil)
	if err != nil {
		return false, fmt.Errorf("failed to enqueue notification trigger: %w", err)
	}
	return !res.UniqueSkippedAsDuplicate, nil
}
