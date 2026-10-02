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

// deliveryWorkerStore is the storage surface DeliveryWorker needs. Satisfied
// structurally by *storage.Storage once its notification_deliveries methods
// exist (a separate, concurrently-developed task) — this file does not
// import the storage package directly.
type deliveryWorkerStore interface {
	GetNotificationDeliveryByRiverJobID(ctx context.Context, orgID int, riverJobID int64) (*notificationdelivery.NotificationDelivery, error)
	UpdateNotificationDeliveryState(ctx context.Context, orgID int, id int64, state notificationdelivery.State, errorKind, providerMessageID *string) error
}

// DeliveryWorker works one DeliveryJobArgs job by loading its audit row,
// calling the channel adapter, and recording the outcome.
//
// DeliveryJobArgs itself is defined in job.go, part of a sibling task
// (Enqueue) landing concurrently. If job.go does not yet exist in this
// worktree when you start, create this minimal version of it here so your
// own package compiles and tests pass in isolation — then when the sibling
// task's branch merges, there will be two definitions and one must be
// deleted (prefer keeping the sibling's, since Enqueue also needs it):
//
//	type DeliveryJobArgs struct {
//	    OrgID                  int   `json:"org_id"`
//	    NotificationDeliveryID int64 `json:"notification_delivery_id"`
//	}
//	func (DeliveryJobArgs) Kind() string { return "notification_delivery" }
//
// Check first: run `find . -name job.go` under backend/internal/notification/outbox/
// in your worktree. If it's not there, add the above as job.go. If it is
// there already (e.g. because you merged in a sibling branch), do not
// duplicate it.
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
		state = notificationdelivery.StateInFlight
		kind := string(ErrorTransient)
		errorKindPtr = &kind
	}

	if updateErr := w.store.UpdateNotificationDeliveryState(ctx, job.Args.OrgID, delivery.ID, state, errorKindPtr, providerMessageIDPtr); updateErr != nil {
		return updateErr
	}

	if outcome != DeliveryDelivered {
		// Returning the original error (not a wrapped one) lets River's own
		// retry/discard logic classify it the same way for every job kind.
		return sendErr
	}
	return nil
}
