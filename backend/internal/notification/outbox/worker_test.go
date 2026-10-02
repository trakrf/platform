package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/notification/outbox"
)

func TestRetryLadder_FollowsFixedDelaysThenCapsAtLastValue(t *testing.T) {
	ladder := outbox.NewRetryLadder()
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	next := ladder.NextRetry(&rivertype.JobRow{Attempt: 1, AttemptedAt: &base})
	require.WithinDuration(t, base.Add(1*time.Second), next, time.Millisecond)

	next5 := ladder.NextRetry(&rivertype.JobRow{Attempt: 5, AttemptedAt: &base})
	next6 := ladder.NextRetry(&rivertype.JobRow{Attempt: 6, AttemptedAt: &base})
	require.Equal(t, next5, next6, "beyond the ladder's length, the last rung's delay repeats rather than panicking or growing unbounded")
}

type fakeAdapter struct {
	result outbox.Result
	err    error
}

func (f fakeAdapter) Send(ctx context.Context, cmd outbox.Command) (outbox.Result, error) {
	return f.result, f.err
}

func TestDeliveryWorker_AmbiguousOutcomeIsNotTreatedAsSafeToBlindlyRetry(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	adapter := fakeAdapter{err: &outbox.ProviderError{Kind: outbox.ErrorOutcomeUnknown}}
	worker := outbox.NewDeliveryWorker(nil, adapter, metrics)

	outcome := outbox.ClassifyOutcome(adapter.err)
	require.Equal(t, outbox.DeliveryRetryableFailure, outcome, "an ambiguous provider outcome must still be retried through the provider's own idempotency key, not silently marked delivered or permanently failed")
	_ = worker
}
