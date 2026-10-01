package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
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

// fakeDeliveryWorkerStore is a test double for the unexported
// deliveryWorkerStore interface DeliveryWorker.Work depends on. It records
// every UpdateNotificationDeliveryState call so tests can assert the exact
// state/error-kind/provider-message-id the worker decided to persist,
// without a real database.
type fakeDeliveryWorkerStore struct {
	delivery *notificationdelivery.NotificationDelivery
	getErr   error

	updateCalls          int
	gotState             notificationdelivery.State
	gotErrorKind         *string
	gotProviderMessageID *string
	updateErr            error
}

func (s *fakeDeliveryWorkerStore) GetNotificationDeliveryByRiverJobID(ctx context.Context, orgID int, riverJobID int64) (*notificationdelivery.NotificationDelivery, error) {
	return s.delivery, s.getErr
}

func (s *fakeDeliveryWorkerStore) UpdateNotificationDeliveryState(ctx context.Context, orgID int, id int64, state notificationdelivery.State, errorKind, providerMessageID *string) error {
	s.updateCalls++
	s.gotState = state
	s.gotErrorKind = errorKind
	s.gotProviderMessageID = providerMessageID
	return s.updateErr
}

func newTestJob(attempt, maxAttempts int) *river.Job[outbox.DeliveryJobArgs] {
	return &river.Job[outbox.DeliveryJobArgs]{
		JobRow: &rivertype.JobRow{
			ID:          99,
			Attempt:     attempt,
			MaxAttempts: maxAttempts,
		},
		Args: outbox.DeliveryJobArgs{OrgID: 42, NotificationDeliveryID: 7},
	}
}

func TestDeliveryWorker_AmbiguousOutcomeIsNotTreatedAsSafeToBlindlyRetry(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	adapter := fakeAdapter{err: &outbox.ProviderError{Kind: outbox.ErrorOutcomeUnknown}}

	outcome := outbox.ClassifyOutcome(adapter.err)
	require.Equal(t, outbox.DeliveryRetryableFailure, outcome, "an ambiguous provider outcome must still be retried through the provider's own idempotency key, not silently marked delivered or permanently failed")

	store := &fakeDeliveryWorkerStore{
		delivery: &notificationdelivery.NotificationDelivery{ID: 7, DeliveryID: "evt-1:c1:email", Channel: notificationdelivery.ChannelEmail},
	}
	worker := outbox.NewDeliveryWorker(store, adapter, metrics)

	// Not the last attempt: the worker must leave the audit row in flight
	// (River will retry) and record the real ambiguous kind, not a hardcoded
	// "transient" that would erase the distinction support needs between
	// "definitely not sent" and "may already have been accepted."
	workErr := worker.Work(context.Background(), newTestJob(1, 6))
	require.ErrorIs(t, workErr, adapter.err, "a retryable outcome's original error must be returned so River's own retry/discard logic classifies it")

	require.Equal(t, 1, store.updateCalls)
	require.Equal(t, notificationdelivery.StateInFlight, store.gotState, "an ambiguous outcome with attempts remaining must stay in_flight so River retries it")
	require.NotNil(t, store.gotErrorKind)
	require.Equal(t, string(outbox.ErrorOutcomeUnknown), *store.gotErrorKind, "the audit row must record the real provider error kind, not a hardcoded 'transient'")
}

func TestDeliveryWorker_PermanentFailureCancelsRiverJobInsteadOfRetrying(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	sendErr := &outbox.ProviderError{Kind: outbox.ErrorPermanent}
	adapter := fakeAdapter{err: sendErr}
	store := &fakeDeliveryWorkerStore{
		delivery: &notificationdelivery.NotificationDelivery{ID: 7, DeliveryID: "evt-2:c1:email", Channel: notificationdelivery.ChannelEmail},
	}
	worker := outbox.NewDeliveryWorker(store, adapter, metrics)

	// First attempt of 6: plenty of attempts remain, but a permanent failure
	// must never be retried regardless of how many attempts are left.
	workErr := worker.Work(context.Background(), newTestJob(1, 6))

	require.Equal(t, 1, store.updateCalls)
	require.Equal(t, notificationdelivery.StatePermanentlyFailed, store.gotState, "a permanent failure must be recorded as permanently_failed in our own audit table")
	require.NotNil(t, store.gotErrorKind)
	require.Equal(t, string(outbox.ErrorPermanent), *store.gotErrorKind)

	var cancelErr *river.JobCancelError
	require.True(t, errors.As(workErr, &cancelErr), "Work must return river.JobCancel(sendErr) for a permanent failure so River stops retrying instead of running the job up to 5 more times")
}

func TestDeliveryWorker_ExhaustedRetryableFailureMarksAuditRowPermanentlyFailed(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := outbox.NewMetrics(registry)
	require.NoError(t, err)

	sendErr := &outbox.ProviderError{Kind: outbox.ErrorTransient}
	adapter := fakeAdapter{err: sendErr}
	store := &fakeDeliveryWorkerStore{
		delivery: &notificationdelivery.NotificationDelivery{ID: 7, DeliveryID: "evt-3:c1:email", Channel: notificationdelivery.ChannelEmail},
	}
	worker := outbox.NewDeliveryWorker(store, adapter, metrics)

	// Attempt 6 of 6 (MaxAttempts): this is the job's last try. River will
	// discard the job itself after this call returns an error (Attempt >=
	// MaxAttempts), but nothing will ever update our own audit row again
	// after that, so Work must finalize it as permanently_failed now rather
	// than leaving it stuck at in_flight forever.
	workErr := worker.Work(context.Background(), newTestJob(6, 6))
	require.ErrorIs(t, workErr, sendErr, "the last attempt still returns the plain error so River's own discard logic (Attempt >= MaxAttempts) applies normally")

	var cancelErr *river.JobCancelError
	require.False(t, errors.As(workErr, &cancelErr), "a retryable-kind failure must not be wrapped in JobCancel — River's own Attempt>=MaxAttempts check is what discards it")

	require.Equal(t, 1, store.updateCalls)
	require.Equal(t, notificationdelivery.StatePermanentlyFailed, store.gotState, "the audit row must mirror River's discarded state as permanently_failed, not be left stuck at in_flight")
	require.NotNil(t, store.gotErrorKind)
	require.Equal(t, string(outbox.ErrorTransient), *store.gotErrorKind)
}
