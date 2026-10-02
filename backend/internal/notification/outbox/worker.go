package outbox

import (
	"context"
	"errors"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
)

// retryLadder is the TRA-398-designed backoff schedule, carried forward per
// TRA-1271's instruction to reuse it rather than redesign retry timing.
var retryLadder = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	30 * time.Second,
	5 * time.Minute,
	30 * time.Minute,
}

// retryPolicy implements river.ClientRetryPolicy with the fixed ladder
// above instead of River's default exponential backoff.
type retryPolicy struct{}

// NewRetryLadder returns the TRA-1192 client-level retry policy.
func NewRetryLadder() river.ClientRetryPolicy {
	return retryPolicy{}
}

func (retryPolicy) NextRetry(job *rivertype.JobRow) time.Time {
	idx := job.Attempt - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(retryLadder) {
		idx = len(retryLadder) - 1
	}
	base := time.Now().UTC()
	if job.AttemptedAt != nil {
		base = *job.AttemptedAt
	}
	return base.Add(retryLadder[idx])
}

// ClassifyOutcome maps a ChannelAdapter error to a DeliveryOutcome. An
// outcome-unknown error (the provider may have already accepted the send)
// is classified as retryable, not permanent: a retry after an ambiguous
// submission must still go through the provider's own idempotency key
// rather than being treated as either "safe, resend freely" or "unsafe,
// give up."
func ClassifyOutcome(err error) DeliveryOutcome {
	if err == nil {
		return DeliveryDelivered
	}
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		switch providerErr.Kind {
		case ErrorPermanent:
			return DeliveryPermanentFailure
		case ErrorTransient, ErrorOutcomeUnknown:
			return DeliveryRetryableFailure
		}
	}
	return DeliveryRetryableFailure
}

// deliveryWorkerStore is the storage surface DeliveryWorker needs, narrowed
// for testing. Satisfied structurally by *storage.Storage — this file does
// not import the storage package directly.
type deliveryWorkerStore interface {
	GetNotificationDeliveryByRiverJobID(ctx context.Context, orgID int, riverJobID int64) (*notificationdelivery.NotificationDelivery, error)
	UpdateNotificationDeliveryState(ctx context.Context, orgID int, id int64, state notificationdelivery.State, errorKind, providerMessageID *string) error
}

// DeliveryWorker works one DeliveryJobArgs job by loading its audit row,
// calling the channel adapter, and recording the outcome.
type DeliveryWorker struct {
	river.WorkerDefaults[DeliveryJobArgs]
	store   deliveryWorkerStore
	adapter ChannelAdapter
	metrics *Metrics
}

func NewDeliveryWorker(store deliveryWorkerStore, adapter ChannelAdapter, metrics *Metrics) *DeliveryWorker {
	return &DeliveryWorker{store: store, adapter: adapter, metrics: metrics}
}

func (w *DeliveryWorker) Work(ctx context.Context, job *river.Job[DeliveryJobArgs]) error {
	delivery, err := w.store.GetNotificationDeliveryByRiverJobID(ctx, job.Args.OrgID, job.ID)
	if err != nil {
		return err
	}

	cmd := Command{
		DeliveryID: delivery.DeliveryID,
		OrgID:      job.Args.OrgID,
		Channel:    delivery.Channel,
	}

	result, sendErr := w.adapter.Send(ctx, cmd)
	outcome := ClassifyOutcome(sendErr)
	w.metrics.RecordDelivery(outcome)

	var state notificationdelivery.State
	var errorKindPtr *string
	var providerMessageIDPtr *string
	switch outcome {
	case DeliveryDelivered:
		state = notificationdelivery.StateDelivered
		providerMessageIDPtr = &result.ProviderMessageID
	case DeliveryPermanentFailure:
		state = notificationdelivery.StatePermanentlyFailed
		kind := string(ErrorPermanent)
		errorKindPtr = &kind
	default: // DeliveryRetryableFailure
		// River discards a job itself once Attempt reaches MaxAttempts; no
		// further call into this worker will ever happen for this job. Mirror
		// that end state in our own audit row now instead of leaving it
		// stuck at in_flight forever.
		if job.Attempt >= job.MaxAttempts {
			state = notificationdelivery.StatePermanentlyFailed
		} else {
			state = notificationdelivery.StateInFlight
		}
		// Propagate the real provider error kind (transient vs.
		// outcome-unknown) rather than hardcoding transient: this is the
		// distinction support needs between "definitely not sent" and "may
		// have already been accepted."
		kind := string(ErrorTransient)
		var providerErr *ProviderError
		if errors.As(sendErr, &providerErr) && providerErr.Kind != "" {
			kind = string(providerErr.Kind)
		}
		errorKindPtr = &kind
	}

	if updateErr := w.store.UpdateNotificationDeliveryState(ctx, job.Args.OrgID, delivery.ID, state, errorKindPtr, providerMessageIDPtr); updateErr != nil {
		return updateErr
	}

	switch outcome {
	case DeliveryDelivered:
		return nil
	case DeliveryPermanentFailure:
		// A permanent failure must never be retried, no matter how many
		// attempts remain. river.JobCancel tells River's client to stop here
		// instead of treating this as an ordinary retryable error, which
		// would otherwise retry up to 5 more times over ~36 minutes per the
		// retry ladder even though the audit row already says
		// permanently_failed.
		return river.JobCancel(sendErr)
	default:
		// Returning the original error (not a wrapped one) lets River's own
		// retry/discard logic classify it the same way for every job kind.
		return sendErr
	}
}
