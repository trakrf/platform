package geofence

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/detection"
	"github.com/trakrf/platform/backend/internal/storage"
)

var (
	_ detection.Stage = (*Stage)(nil)
	_ alarmTxStore    = (*storage.Storage)(nil)
	_ alarmRecorder   = (*detection.Recorder)(nil)
)

// alarmTxStore writes an alarm on the caller's transaction; *storage.Storage
// satisfies it.
type alarmTxStore interface {
	InsertAlarmEventTx(ctx context.Context, tx pgx.Tx, orgID int, ev storage.AlarmEventRow) (int, error)
}

// alarmRecorder queues an alarm's trigger on the caller's transaction;
// *detection.Recorder satisfies it.
type alarmRecorder interface {
	RecordAlarm(ctx context.Context, tx pgx.Tx, orgID, alarmID int) error
}

// Stage is the geofence detector for the MQTT subscriber. It decides and writes
// alarms inside the scan transaction, queues their triggers there, and drives
// outputs only once the scan outcome is known.
//
// This is the only place a trigger is enqueued. Simulated reads reach the same
// engine through Evaluate, which must never enqueue: the simulator does not create
// triggers.
//
// A subscriber rollback can race a simulator call on the shared engine; this is accepted.
type Stage struct {
	engine *Engine
	store  alarmTxStore
	rec    alarmRecorder
}

// Stage builds the detection stage for this engine.
func (e *Engine) Stage(store alarmTxStore, rec alarmRecorder) *Stage {
	return &Stage{engine: e, store: store, rec: rec}
}

// Name identifies the stage in logs and metrics.
func (s *Stage) Name() string { return "geofence" }

// Detect writes an alarm and its trigger for every fire. On a write error it stops
// writing and returns that error with a Pending covering every fire, written or
// not: the pipeline rolls back this stage's rows, but the outputs must still drive.
// A panic while writing is treated the same way: decide has already changed latch,
// pulse and presence state, and only this Pending can settle it.
// The deferred drives use ctx, so they stay inside the message's time limit, as
// they do in Evaluate.
func (s *Stage) Detect(ctx context.Context, tx pgx.Tx, in storage.DetectInput) (pending detection.Pending, err error) {
	fires, refreshes := s.engine.decide(ctx, in.OrgID, in.ReceivedAt, in.Reads)
	if len(fires) == 0 && len(refreshes) == 0 {
		return detection.Pending{}, nil
	}
	// A presence refresh is not an alarm: it writes nothing and has nothing to
	// undo, so it is sent whether the scan commits or rolls back. Either way the
	// tag was in view, and a missed refresh lets the output time out.
	refresh := func() { s.engine.refreshPresence(ctx, in.OrgID, in.ReceivedAt, refreshes) }
	pending = detection.Pending{
		OnCommit:   func(rowsKept bool) { s.settle(ctx, in.OrgID, fires, rowsKept); refresh() },
		OnRollback: func() { undoAll(fires); refresh() },
	}
	if len(fires) == 0 {
		return pending, nil
	}
	defer func() {
		if p := recover(); p != nil {
			metricEventWriteErrors.Inc()
			err = fmt.Errorf("alarm write panicked: %v\n%s", p, debug.Stack())
		}
	}()
	return pending, s.write(ctx, tx, in, fires)
}

func (s *Stage) write(ctx context.Context, tx pgx.Tx, in storage.DetectInput, fires []fire) error {
	for _, f := range fires {
		err := s.writeFire(ctx, tx, in, f)
		if err != nil {
			s.engine.log.Error().Err(err).Int("org_id", in.OrgID).Str("epc", f.read.EPC).Msg("alarm write failed; the scan's alarms are discarded")
			metricEventWriteErrors.Inc()
			return err
		}
	}
	return nil
}

func (s *Stage) writeFire(ctx context.Context, tx pgx.Tx, in storage.DetectInput, f fire) error {
	id, err := s.store.InsertAlarmEventTx(ctx, tx, in.OrgID, s.engine.alarmRow(f, in.TagScanID, in.ReceivedAt))
	if err != nil {
		return err
	}
	return s.rec.RecordAlarm(ctx, tx, in.OrgID, id)
}

// settle runs after the scan commits. rowsKept is the only signal that the alarms
// survived: the pipeline can discard them even when Detect returned nil. Without
// them a fire with drives still drives, and a fire with none is undone, so its
// next read can record the alarm instead of it being lost for the re-arm window.
func (s *Stage) settle(ctx context.Context, orgID int, fires []fire, rowsKept bool) {
	for _, f := range fires {
		if !rowsKept && len(f.drives) == 0 {
			f.undo()
			continue
		}
		s.engine.driveFire(ctx, orgID, f)
		if rowsKept {
			s.engine.noteFired(orgID, f)
		}
	}
}

// undoAll takes back every fire of a rolled-back scan, newest first.
func undoAll(fires []fire) {
	for i := len(fires) - 1; i >= 0; i-- {
		fires[i].undo()
	}
}
