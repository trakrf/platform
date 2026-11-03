package outbox_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
)

type recordingAdapter struct {
	calls  int
	got    outbox.Command
	result outbox.Result
	err    error
}

func (a *recordingAdapter) Send(ctx context.Context, cmd outbox.Command) (outbox.Result, error) {
	a.calls++
	a.got = cmd
	return a.result, a.err
}

type reasonErr struct {
	kind   outbox.ErrorKind
	reason string
}

func (e reasonErr) Error() string       { return "test adapter failure" }
func (e reasonErr) ErrorReason() string { return e.reason }
func (e reasonErr) Unwrap() error       { return &outbox.ProviderError{Kind: e.kind} }

func newSendWorker(t *testing.T, state notificationdelivery.State, payload []byte, adapter *recordingAdapter) (*outbox.DeliveryWorker, *fakeDeliveryWorkerStore) {
	t.Helper()
	worker, store, _ := newSendWorkerWithRegistry(t, state, payload, adapter)
	return worker, store
}

func newSendWorkerWithRegistry(t *testing.T, state notificationdelivery.State, payload []byte, adapter *recordingAdapter) (*outbox.DeliveryWorker, *fakeDeliveryWorkerStore, *prometheus.Registry) {
	t.Helper()
	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)
	store := &fakeDeliveryWorkerStore{delivery: &notificationdelivery.NotificationDelivery{
		ID: 7, DeliveryID: "d-1", Channel: notificationdelivery.ChannelEmail, State: state, Payload: payload,
	}}
	return outbox.NewDeliveryWorker(store, adapter, metrics), store, registry
}

// counterTotal sums every counter sample in the registry.
func counterTotal(t *testing.T, registry *prometheus.Registry) float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	var total float64
	for _, family := range families {
		for _, m := range family.GetMetric() {
			if m.GetCounter() != nil {
				total += m.GetCounter().GetValue()
			}
		}
	}
	return total
}

func TestDeliveryWorker_CommandCarriesSavedPayload(t *testing.T) {
	adapter := &recordingAdapter{}
	worker, _ := newSendWorker(t, notificationdelivery.StatePending, []byte(`{"to":"a@b.test"}`), adapter)

	require.NoError(t, worker.Work(context.Background(), newTestJob(1, 6)))
	require.Equal(t, `{"to":"a@b.test"}`, string(adapter.got.Payload))
	require.Equal(t, "d-1", adapter.got.DeliveryID)
	require.Equal(t, 42, adapter.got.OrgID)
}

func TestDeliveryWorker_SkipsFinishedRows(t *testing.T) {
	for _, state := range []notificationdelivery.State{notificationdelivery.StateDelivered, notificationdelivery.StatePermanentlyFailed} {
		t.Run(string(state), func(t *testing.T) {
			adapter := &recordingAdapter{}
			worker, store, registry := newSendWorkerWithRegistry(t, state, []byte(`{}`), adapter)

			require.NoError(t, worker.Work(context.Background(), newTestJob(1, 6)))
			require.Equal(t, 0, adapter.calls)
			require.Equal(t, 0, store.updateCalls)
			require.Zero(t, counterTotal(t, registry), "a skipped row must record no metric")
		})
	}
}

func TestDeliveryWorker_PermanentFailureRecordsReason(t *testing.T) {
	adapter := &recordingAdapter{err: reasonErr{kind: outbox.ErrorPermanent, reason: "opted_out"}}
	worker, store := newSendWorker(t, notificationdelivery.StatePending, []byte(`{}`), adapter)

	workErr := worker.Work(context.Background(), newTestJob(1, 6))
	var cancelErr *river.JobCancelError
	require.True(t, errors.As(workErr, &cancelErr), "permanent failure must cancel the River job")
	require.Equal(t, notificationdelivery.StatePermanentlyFailed, store.gotState)
	require.NotNil(t, store.gotErrorKind)
	require.Equal(t, "opted_out", *store.gotErrorKind)
}

func TestDeliveryWorker_PermanentFailureWithoutReason(t *testing.T) {
	adapter := &recordingAdapter{err: &outbox.ProviderError{Kind: outbox.ErrorPermanent}}
	worker, store := newSendWorker(t, notificationdelivery.StatePending, []byte(`{}`), adapter)

	require.Error(t, worker.Work(context.Background(), newTestJob(1, 6)))
	require.NotNil(t, store.gotErrorKind)
	require.Equal(t, "permanent", *store.gotErrorKind)
}

func TestDeliveryWorker_RetryableFailureKeepsKind(t *testing.T) {
	adapter := &recordingAdapter{err: &outbox.ProviderError{Kind: outbox.ErrorOutcomeUnknown}}
	worker, store := newSendWorker(t, notificationdelivery.StatePending, []byte(`{}`), adapter)

	require.Error(t, worker.Work(context.Background(), newTestJob(1, 6)))
	require.Equal(t, notificationdelivery.StateInFlight, store.gotState)
	require.NotNil(t, store.gotErrorKind)
	require.Equal(t, "outcome_unknown", *store.gotErrorKind)
}

func TestDeliveryWorker_PendingAndInFlightRowsAreSent(t *testing.T) {
	for _, state := range []notificationdelivery.State{notificationdelivery.StatePending, notificationdelivery.StateInFlight} {
		t.Run(string(state), func(t *testing.T) {
			adapter := &recordingAdapter{result: outbox.Result{ProviderMessageID: "pm-1"}}
			worker, store, registry := newSendWorkerWithRegistry(t, state, []byte(`{}`), adapter)

			require.NoError(t, worker.Work(context.Background(), newTestJob(1, 6)))
			require.Equal(t, 1, adapter.calls)
			require.Equal(t, notificationdelivery.StateDelivered, store.gotState)
			require.NotNil(t, store.gotProviderMessageID)
			require.Equal(t, "pm-1", *store.gotProviderMessageID)
			require.Nil(t, store.gotErrorKind)
			require.Equal(t, float64(1), counterTotal(t, registry))
		})
	}
}

func TestDeliveryWorker_FailureRecording(t *testing.T) {
	boom := errors.New("boom")
	wrapped := fmt.Errorf("adapter: %w", reasonErr{kind: outbox.ErrorPermanent, reason: "opted_out"})
	retryReason := reasonErr{kind: outbox.ErrorTransient, reason: "x"}

	tests := []struct {
		name      string
		err       error
		attempt   int
		wantState notificationdelivery.State
		wantKind  string
		wantCancel bool
	}{
		{"reason found through wrapping", wrapped, 1, notificationdelivery.StatePermanentlyFailed, "opted_out", true},
		{"reason on retryable branch", retryReason, 1, notificationdelivery.StateInFlight, "x", false},
		{"retryable reason on last attempt", retryReason, 6, notificationdelivery.StatePermanentlyFailed, "x", false},
		{"empty reason falls through to kind", reasonErr{kind: outbox.ErrorOutcomeUnknown}, 1, notificationdelivery.StateInFlight, "outcome_unknown", false},
		{"non-provider error is transient", boom, 1, notificationdelivery.StateInFlight, "transient", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &recordingAdapter{err: tt.err}
			worker, store := newSendWorker(t, notificationdelivery.StatePending, []byte(`{}`), adapter)

			workErr := worker.Work(context.Background(), newTestJob(tt.attempt, 6))

			require.Error(t, workErr)
			var cancelErr *river.JobCancelError
			require.Equal(t, tt.wantCancel, errors.As(workErr, &cancelErr))
			if !tt.wantCancel {
				require.Equal(t, tt.err, workErr, "the original error must be returned for River's own retry logic")
			}
			require.Equal(t, 1, store.updateCalls)
			require.Equal(t, tt.wantState, store.gotState)
			require.NotNil(t, store.gotErrorKind)
			require.Equal(t, tt.wantKind, *store.gotErrorKind)
			require.Nil(t, store.gotProviderMessageID)
		})
	}
}

func TestDeliveryWorker_GetErrorIsReturnedWithoutSendOrUpdate(t *testing.T) {
	adapter := &recordingAdapter{}
	worker, store := newSendWorker(t, notificationdelivery.StatePending, nil, adapter)
	getErr := errors.New("get failed")
	store.getErr = getErr

	require.ErrorIs(t, worker.Work(context.Background(), newTestJob(1, 6)), getErr)
	require.Equal(t, 0, adapter.calls)
	require.Equal(t, 0, store.updateCalls)
}

func TestDeliveryWorker_UpdateErrorAfterSuccessfulSendIsReturned(t *testing.T) {
	adapter := &recordingAdapter{result: outbox.Result{ProviderMessageID: "pm-1"}}
	worker, store := newSendWorker(t, notificationdelivery.StatePending, []byte(`{}`), adapter)
	updateErr := errors.New("update failed")
	store.updateErr = updateErr

	require.ErrorIs(t, worker.Work(context.Background(), newTestJob(1, 6)), updateErr, "River must retry so the audit row catches up")
	require.Equal(t, 1, adapter.calls)
	require.Equal(t, 1, store.updateCalls)
}
