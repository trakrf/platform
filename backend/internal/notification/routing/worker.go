package routing

import (
	"context"
	"errors"

	"github.com/riverqueue/river"
)

// TriggerPlanner plans the deliveries for one trigger. *DeliveryPlanner
// satisfies it.
type TriggerPlanner interface {
	Plan(ctx context.Context, t Trigger) (Result, error)
}

var _ TriggerPlanner = (*DeliveryPlanner)(nil)

// TriggerWorker runs the planner for each queued trigger job.
type TriggerWorker struct {
	river.WorkerDefaults[TriggerJobArgs]
	planner TriggerPlanner
}

func NewTriggerWorker(p TriggerPlanner) *TriggerWorker {
	return &TriggerWorker{planner: p}
}

// Work cancels a job whose trigger can never be planned and returns any other
// error so River retries it.
func (w *TriggerWorker) Work(ctx context.Context, job *river.Job[TriggerJobArgs]) error {
	if _, err := w.planner.Plan(ctx, job.Args.Trigger()); err != nil {
		if errors.Is(err, ErrInvalidTrigger) {
			return river.JobCancel(err)
		}
		return err
	}
	return nil
}

// RegisterTriggerWorker returns a hook for outbox.NewRuntime that adds the
// trigger worker to the runtime's workers.
func RegisterTriggerWorker(p TriggerPlanner) func(*river.Workers) {
	return func(w *river.Workers) {
		river.AddWorker(w, NewTriggerWorker(p))
	}
}
