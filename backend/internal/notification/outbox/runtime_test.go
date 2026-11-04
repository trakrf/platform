//go:build integration

package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestRuntime_StartStop_NoPanicNoLeak(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	runtime, err := outbox.NewRuntime(db.AppPool, fakeAdapter{}, db.Store, metrics)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, runtime.Start(ctx))
	require.NoError(t, runtime.Stop(ctx))
}

func TestNewRuntime_RegistersExtraWorkers(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	metrics, err := outbox.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)

	calls := 0
	_, err = outbox.NewRuntime(db.AppPool, fakeAdapter{}, db.Store, metrics, func(w *river.Workers) {
		calls++
		require.NotNil(t, w)
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

// slowJobArgs is a test-only job kind whose worker never finishes by itself.
type slowJobArgs struct{}

func (slowJobArgs) Kind() string { return "outbox_runtime_test_slow" }

type slowWorker struct {
	river.WorkerDefaults[slowJobArgs]
	started chan struct{}
	ctxErr  chan error
}

func (w *slowWorker) Work(ctx context.Context, _ *river.Job[slowJobArgs]) error {
	close(w.started)
	<-ctx.Done()
	w.ctxErr <- ctx.Err()
	return ctx.Err()
}

func TestRuntime_StopCancelsSlowJobAfterSoftStopTimeout(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	metrics, err := outbox.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	worker := &slowWorker{started: make(chan struct{}), ctxErr: make(chan error, 1)}
	runtime, err := outbox.NewRuntime(db.AppPool, fakeAdapter{}, db.Store, metrics, func(w *river.Workers) {
		river.AddWorker(w, worker)
	})
	require.NoError(t, err)

	inserter, err := outbox.NewInserter(db.AppPool)
	require.NoError(t, err)
	_, err = inserter.Insert(context.Background(), slowJobArgs{}, nil)
	require.NoError(t, err)

	require.NoError(t, runtime.Start(context.Background()))
	select {
	case <-worker.started:
	case <-time.After(10 * time.Second):
		t.Fatal("the slow job never started")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	began := time.Now()
	require.NoError(t, runtime.Stop(stopCtx))
	require.Less(t, time.Since(began), 6*time.Second)

	select {
	case err := <-worker.ctxErr:
		require.ErrorIs(t, err, context.Canceled)
	default:
		t.Fatal("the worker did not observe a cancelled context")
	}
}
