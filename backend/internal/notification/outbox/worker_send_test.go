package outbox_test

import (
	"context"
	"errors"
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
	metrics, err := outbox.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	store := &fakeDeliveryWorkerStore{delivery: &notificationdelivery.NotificationDelivery{
		ID: 7, DeliveryID: "d-1", Channel: notificationdelivery.ChannelEmail, State: state, Payload: payload,
	}}
	return outbox.NewDeliveryWorker(store, adapter, metrics), store
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
			worker, store := newSendWorker(t, state, []byte(`{}`), adapter)

			require.NoError(t, worker.Work(context.Background(), newTestJob(1, 6)))
			require.Equal(t, 0, adapter.calls)
			require.Equal(t, 0, store.updateCalls)
		})
	}
}

func TestDeliveryWorker_InFlightRowIsStillSent(t *testing.T) {
	adapter := &recordingAdapter{}
	worker, store := newSendWorker(t, notificationdelivery.StateInFlight, []byte(`{}`), adapter)

	require.NoError(t, worker.Work(context.Background(), newTestJob(2, 6)))
	require.Equal(t, 1, adapter.calls)
	require.Equal(t, notificationdelivery.StateDelivered, store.gotState)
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
