package routing_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/notification/routing"
)

func TestTriggerValidate(t *testing.T) {
	require.NoError(t, routing.Trigger{OrgID: 1, Source: notificationevent.SourceAlarm, SourceID: 2}.Validate())
	require.NoError(t, routing.Trigger{OrgID: 1, Source: notificationevent.SourceLocationChange, SourceID: 2}.Validate())

	for name, tr := range map[string]routing.Trigger{
		"unknown source": {OrgID: 1, Source: "geofence", SourceID: 2},
		"zero org":       {OrgID: 0, Source: notificationevent.SourceAlarm, SourceID: 2},
		"zero source id": {OrgID: 1, Source: notificationevent.SourceAlarm, SourceID: 0},
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, tr.Validate(), routing.ErrInvalidTrigger)
		})
	}
}

func TestTriggerJobArgs_JSON(t *testing.T) {
	b, err := json.Marshal(routing.TriggerJobArgs{OrgID: 1, Source: "alarm", SourceID: 2})
	require.NoError(t, err)
	require.JSONEq(t, `{"org_id":1,"source":"alarm","source_id":2}`, string(b))
	require.Equal(t, `{"org_id":1,"source":"alarm","source_id":2}`, string(b))

	args := routing.TriggerJobArgs{}
	require.Equal(t, "notification_trigger", args.Kind())
	opts := args.InsertOpts()
	require.Equal(t, 6, opts.MaxAttempts)
	require.True(t, opts.UniqueOpts.ByArgs)
}

func TestTriggerJobArgs_Trigger(t *testing.T) {
	got := routing.TriggerJobArgs{OrgID: 1, Source: "alarm", SourceID: 2}.Trigger()
	require.Equal(t, routing.Trigger{OrgID: 1, Source: notificationevent.SourceAlarm, SourceID: 2}, got)
}

type recordingInserter struct{ calls int }

func (r *recordingInserter) InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	r.calls++
	return &rivertype.JobInsertResult{}, nil
}

func TestEnqueueTriggerTx_RejectsInvalid(t *testing.T) {
	jobs := &recordingInserter{}
	inserted, err := routing.EnqueueTriggerTx(context.Background(), nil, jobs,
		routing.Trigger{OrgID: 1, Source: "geofence", SourceID: 2})
	require.ErrorIs(t, err, routing.ErrInvalidTrigger)
	require.False(t, inserted)
	require.Zero(t, jobs.calls)
}
