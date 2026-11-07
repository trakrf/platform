package detection

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/storage"
)

// fakeTx records the SQL sent. failOn makes any statement with that prefix fail.
type fakeTx struct {
	pgx.Tx
	sql    []string
	failOn string
}

func (f *fakeTx) Exec(_ context.Context, q string, _ ...any) (pgconn.CommandTag, error) {
	f.sql = append(f.sql, q)
	if f.failOn != "" && strings.HasPrefix(q, f.failOn) {
		return pgconn.CommandTag{}, errors.New("sql failed: " + q)
	}
	return pgconn.CommandTag{}, nil
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
	assert.Equal(t, []string{
		"SAVEPOINT detection_stage",
		"ROLLBACK TO SAVEPOINT detection_stage",
		"RELEASE SAVEPOINT detection_stage",
		"SAVEPOINT detection_stage",
		"RELEASE SAVEPOINT detection_stage",
	}, tx.sql)
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
	require.NoError(t, r.Detect(context.Background(), &fakeTx{failOn: "SAVEPOINT"}, storage.DetectInput{}))
	assert.False(t, a.called)
	assert.Equal(t, before+1, counterValue(t, metricStageErrors.WithLabelValues("sp_begin_stage")))
	r.Committed()
	assert.Empty(t, a.commits)
}

func TestRun_SavepointReleaseFailureRollsBackAndReportsNotKept(t *testing.T) {
	a := &fakeStage{name: "a"}
	tx := &fakeTx{failOn: "RELEASE"}
	r := newPipe(a).Begin()
	require.NoError(t, r.Detect(context.Background(), tx, storage.DetectInput{}))
	r.Committed()
	assert.Equal(t, []bool{false}, a.commits)
	assert.Contains(t, tx.sql, "ROLLBACK TO SAVEPOINT detection_stage")
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, c.Write(&m))
	return m.GetCounter().GetValue()
}
