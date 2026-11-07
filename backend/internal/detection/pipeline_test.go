package detection

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/storage"
)

type fakeSavepoint struct {
	pgx.Tx
	commitErr  error
	committed  bool
	rolledBack bool
}

func (f *fakeSavepoint) Commit(context.Context) error {
	if f.commitErr == nil {
		f.committed = true
	}
	return f.commitErr
}
func (f *fakeSavepoint) Rollback(context.Context) error { f.rolledBack = true; return nil }

type fakeTx struct {
	pgx.Tx
	beginErr  error
	saves     []*fakeSavepoint
	commitErr error
}

func (f *fakeTx) Begin(context.Context) (pgx.Tx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	sp := &fakeSavepoint{commitErr: f.commitErr}
	f.saves = append(f.saves, sp)
	return sp, nil
}

type fakeStage struct {
	name       string
	err        error
	nilPending bool
	called     bool
	commits    []bool
	rollbacks  int
}

func (s *fakeStage) Name() string { return s.name }
func (s *fakeStage) Detect(context.Context, pgx.Tx, storage.DetectInput) (Pending, error) {
	s.called = true
	if s.nilPending {
		return Pending{}, s.err
	}
	return Pending{
		OnCommit:   func(k bool) { s.commits = append(s.commits, k) },
		OnRollback: func() { s.rollbacks++ },
	}, s.err
}

func newPipe(stages ...Stage) *Pipeline {
	l := zerolog.Nop()
	return NewPipeline(&l, stages...)
}

func TestRun_StageFailureRollsBackOnlyItsSavepoint(t *testing.T) {
	a := &fakeStage{name: "a", err: errors.New("boom")}
	b := &fakeStage{name: "b"}
	tx := &fakeTx{}
	r := newPipe(a, b).Begin()

	require.NoError(t, r.Detect(context.Background(), tx, storage.DetectInput{}))
	require.Len(t, tx.saves, 2)
	assert.True(t, tx.saves[0].rolledBack)
	assert.False(t, tx.saves[0].committed)
	assert.True(t, tx.saves[1].committed)
	assert.False(t, tx.saves[1].rolledBack)
}

func TestRun_CommittedReportsRowsKeptPerStage(t *testing.T) {
	a := &fakeStage{name: "a", err: errors.New("boom")}
	b := &fakeStage{name: "b"}
	r := newPipe(a, b).Begin()
	require.NoError(t, r.Detect(context.Background(), &fakeTx{}, storage.DetectInput{}))
	r.Committed()
	assert.Equal(t, []bool{false}, a.commits)
	assert.Equal(t, []bool{true}, b.commits)
}

func TestRun_RolledBackCallsOnRollback(t *testing.T) {
	a := &fakeStage{name: "a"}
	b := &fakeStage{name: "b", err: errors.New("boom")}
	r := newPipe(a, b).Begin()
	require.NoError(t, r.Detect(context.Background(), &fakeTx{}, storage.DetectInput{}))
	r.RolledBack()
	assert.Equal(t, 1, a.rollbacks)
	assert.Equal(t, 1, b.rollbacks)
	assert.Empty(t, a.commits)
	assert.Empty(t, b.commits)
}

func TestRun_NilCallbacksAreSafe(t *testing.T) {
	a := &fakeStage{name: "a", nilPending: true}
	b := &fakeStage{name: "b", nilPending: true, err: errors.New("boom")}
	r := newPipe(a, b).Begin()
	require.NoError(t, r.Detect(context.Background(), &fakeTx{}, storage.DetectInput{}))
	assert.NotPanics(t, r.Committed)
	assert.NotPanics(t, r.RolledBack)
}

func TestRun_SavepointBeginFailureIsCounted(t *testing.T) {
	a := &fakeStage{name: "sp_begin_stage"}
	before := counterValue(t, metricStageErrors.WithLabelValues("sp_begin_stage"))
	r := newPipe(a).Begin()
	require.NoError(t, r.Detect(context.Background(), &fakeTx{beginErr: errors.New("no savepoint")}, storage.DetectInput{}))
	assert.False(t, a.called)
	assert.Equal(t, before+1, counterValue(t, metricStageErrors.WithLabelValues("sp_begin_stage")))
	r.Committed()
	assert.Empty(t, a.commits)
}

func TestRun_SavepointCommitFailureCountsAsStageFailure(t *testing.T) {
	a := &fakeStage{name: "a"}
	r := newPipe(a).Begin()
	require.NoError(t, r.Detect(context.Background(), &fakeTx{commitErr: errors.New("x")}, storage.DetectInput{}))
	r.Committed()
	assert.Equal(t, []bool{false}, a.commits)
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, c.Write(&m))
	return m.GetCounter().GetValue()
}
