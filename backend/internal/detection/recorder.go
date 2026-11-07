// Package detection records detected location changes and gate alarms together
// with the job that notifies about them.
package detection

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/routing"
)

var enqueueTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "detection_trigger_enqueue_total",
	Help: "Notification trigger enqueue attempts by source and result (ok, duplicate, error).",
}, []string{"source", "result"})

// changeStore is the storage surface RecordChange needs.
type changeStore interface {
	InsertLocationChangeIfAbsentTx(ctx context.Context, tx pgx.Tx, orgID int, c notificationevent.LocationChange) (int, bool, error)
}

// Recorder writes a detected change or alarm and its notification trigger job
// in the caller's transaction, so both exist or neither does.
type Recorder struct {
	store changeStore
	jobs  outbox.JobInserter
}

func NewRecorder(store changeStore, jobs outbox.JobInserter) *Recorder {
	return &Recorder{store: store, jobs: jobs}
}

// RecordChange inserts the location change and queues its trigger. recorded is
// false when the change was already recorded; nothing is queued then.
func (r *Recorder) RecordChange(ctx context.Context, tx pgx.Tx, c notificationevent.LocationChange) (recorded bool, err error) {
	id, inserted, err := r.store.InsertLocationChangeIfAbsentTx(ctx, tx, c.OrgID, c)
	if err != nil {
		return false, err
	}
	if !inserted {
		return false, nil
	}
	err = r.enqueue(ctx, tx, routing.Trigger{OrgID: c.OrgID, Source: notificationevent.SourceLocationChange, SourceID: id})
	if err != nil {
		return false, err
	}
	return true, nil
}

// RecordAlarm queues the trigger for an alarm the caller already wrote.
func (r *Recorder) RecordAlarm(ctx context.Context, tx pgx.Tx, orgID, alarmID int) error {
	return r.enqueue(ctx, tx, routing.Trigger{OrgID: orgID, Source: notificationevent.SourceAlarm, SourceID: alarmID})
}

func (r *Recorder) enqueue(ctx context.Context, tx pgx.Tx, t routing.Trigger) error {
	queued, err := routing.EnqueueTriggerTx(ctx, tx, r.jobs, t)
	source := string(t.Source)
	switch {
	case err != nil:
		enqueueTotal.WithLabelValues(source, "error").Inc()
		return fmt.Errorf("failed to enqueue %s trigger: %w", source, err)
	case !queued:
		enqueueTotal.WithLabelValues(source, "duplicate").Inc()
	default:
		enqueueTotal.WithLabelValues(source, "ok").Inc()
	}
	return nil
}
