//go:build integration

package outbox_test

import (
	"context"
	"testing"

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
