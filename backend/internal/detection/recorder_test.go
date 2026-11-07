package detection

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	dto "github.com/prometheus/client_model/go"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationevent"
	"github.com/trakrf/platform/backend/internal/notification/routing"
)

type fakeStore struct {
	id       int
	inserted bool
	err      error
}

func (f fakeStore) InsertLocationChangeIfAbsentTx(context.Context, pgx.Tx, int, notificationevent.LocationChange) (int, bool, error) {
	return f.id, f.inserted, f.err
}

type fakeJobs struct {
	args      []river.JobArgs
	duplicate bool
	err       error
}

func (f *fakeJobs) InsertTx(_ context.Context, _ pgx.Tx, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.args = append(f.args, args)
	return &rivertype.JobInsertResult{UniqueSkippedAsDuplicate: f.duplicate}, nil
}

func change() notificationevent.LocationChange {
	return notificationevent.LocationChange{OrgID: 7, AssetID: 1, ToLocationID: 2, Origin: notificationevent.OriginReader}
}

func counter(source, result string) float64 {
	var m dto.Metric
	if err := enqueueTotal.WithLabelValues(source, result).Write(&m); err != nil {
		panic(err)
	}
	return m.GetCounter().GetValue()
}

func TestRecordChange_NewRowEnqueuesTrigger(t *testing.T) {
	jobs := &fakeJobs{}
	r := NewRecorder(fakeStore{id: 55, inserted: true}, jobs)

	recorded, err := r.RecordChange(context.Background(), nil, change())
	require.NoError(t, err)
	require.True(t, recorded)
	require.Len(t, jobs.args, 1)
	got := jobs.args[0].(routing.TriggerJobArgs)
	require.Equal(t, routing.TriggerJobArgs{OrgID: 7, Source: string(notificationevent.SourceLocationChange), SourceID: 55}, got)
}

func TestRecordChange_ReplayEnqueuesNothing(t *testing.T) {
	jobs := &fakeJobs{}
	r := NewRecorder(fakeStore{inserted: false}, jobs)

	recorded, err := r.RecordChange(context.Background(), nil, change())
	require.NoError(t, err)
	require.False(t, recorded)
	require.Empty(t, jobs.args)
}

func TestRecordChange_InsertErrorReturned(t *testing.T) {
	boom := errors.New("boom")
	jobs := &fakeJobs{}
	r := NewRecorder(fakeStore{err: boom}, jobs)

	recorded, err := r.RecordChange(context.Background(), nil, change())
	require.ErrorIs(t, err, boom)
	require.False(t, recorded)
	require.Empty(t, jobs.args)
}

func TestRecordAlarm_EnqueuesAlarmTrigger(t *testing.T) {
	jobs := &fakeJobs{}
	r := NewRecorder(fakeStore{}, jobs)

	require.NoError(t, r.RecordAlarm(context.Background(), nil, 7, 99))
	require.Len(t, jobs.args, 1)
	require.Equal(t, routing.TriggerJobArgs{OrgID: 7, Source: string(notificationevent.SourceAlarm), SourceID: 99}, jobs.args[0])
}

func TestRecord_CountsEnqueueResults(t *testing.T) {
	ctx := context.Background()
	okBefore := counter("alarm", "ok")
	dupBefore := counter("alarm", "duplicate")
	errBefore := counter("alarm", "error")

	require.NoError(t, NewRecorder(fakeStore{}, &fakeJobs{}).RecordAlarm(ctx, nil, 7, 1))
	require.NoError(t, NewRecorder(fakeStore{}, &fakeJobs{duplicate: true}).RecordAlarm(ctx, nil, 7, 1))
	require.Error(t, NewRecorder(fakeStore{}, &fakeJobs{err: errors.New("x")}).RecordAlarm(ctx, nil, 7, 1))

	require.Equal(t, okBefore+1, counter("alarm", "ok"))
	require.Equal(t, dupBefore+1, counter("alarm", "duplicate"))
	require.Equal(t, errBefore+1, counter("alarm", "error"))

	chOK := counter("location_change", "ok")
	_, err := NewRecorder(fakeStore{id: 3, inserted: true}, &fakeJobs{}).RecordChange(ctx, nil, change())
	require.NoError(t, err)
	require.Equal(t, chOK+1, counter("location_change", "ok"))
}
