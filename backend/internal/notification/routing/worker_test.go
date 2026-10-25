package routing_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/notification/routing"
)

type fakeTriggerPlanner struct {
	got routing.Trigger
	err error
}

func (f *fakeTriggerPlanner) Plan(_ context.Context, t routing.Trigger) (routing.Result, error) {
	f.got = t
	return routing.Result{}, f.err
}

func triggerJob(orgID int, source string, sourceID int) *river.Job[routing.TriggerJobArgs] {
	return &river.Job[routing.TriggerJobArgs]{
		Args: routing.TriggerJobArgs{OrgID: orgID, Source: source, SourceID: sourceID},
	}
}

func TestWork_InvalidTriggerIsCancelled(t *testing.T) {
	planner := &fakeTriggerPlanner{err: fmt.Errorf("plan: %w", routing.ErrInvalidTrigger)}
	err := routing.NewTriggerWorker(planner).Work(context.Background(), triggerJob(1, "alarm", 2))

	var cancel *rivertype.JobCancelError
	require.True(t, errors.As(err, &cancel), "got %v", err)
}

func TestWork_OtherErrorIsRetried(t *testing.T) {
	boom := errors.New("database hiccup")
	planner := &fakeTriggerPlanner{err: boom}
	err := routing.NewTriggerWorker(planner).Work(context.Background(), triggerJob(1, "alarm", 2))

	require.ErrorIs(t, err, boom)
	var cancel *rivertype.JobCancelError
	require.False(t, errors.As(err, &cancel))
}

func TestWork_PassesTrigger(t *testing.T) {
	planner := &fakeTriggerPlanner{}
	err := routing.NewTriggerWorker(planner).Work(context.Background(), triggerJob(1, "location_change", 9))

	require.NoError(t, err)
	require.Equal(t, routing.Trigger{OrgID: 1, Source: notificationevent.SourceLocationChange, SourceID: 9}, planner.got)
}
