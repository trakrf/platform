// Package outbox is the durable, provider-neutral notification delivery
// outbox (TRA-1192). It owns the transactional enqueue contract and the
// generic worker; it does NOT call any real provider. The adapter that wires
// ChannelAdapter to the email and SMS senders lives in notification/dispatch.
package outbox

import (
	"context"
	"fmt"

	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
)

// Command is a provider-neutral request to deliver one notification.
// DeliveryID must be the stable event x recipient x channel identity from
// TRA-1271, unique within OrgID.
type Command struct {
	DeliveryID string
	OrgID      int
	Channel    notificationdelivery.Channel
	// Routing context recorded on the delivery row. A zero value is stored
	// as NULL.
	EventID     int
	RecipientID int
	AssetID     int
	// Payload is the rendered message (raw JSON). The batch enqueue path
	// persists it to notification_deliveries.payload; it contains PII, so it
	// must never be put in River job args or logs.
	Payload []byte
}

// Result reports a channel adapter's successful submission.
type Result struct {
	ProviderMessageID string
}

// ErrorKind classifies a channel adapter failure into stable handling
// categories, mirroring email.ErrorKind / sms.ErrorKind.
type ErrorKind string

const (
	ErrorTransient      ErrorKind = "transient"
	ErrorPermanent      ErrorKind = "permanent"
	ErrorOutcomeUnknown ErrorKind = "outcome_unknown"
)

// ProviderError never retains payload or provider response text — only the
// bounded classification needed to decide retry behavior.
type ProviderError struct {
	Kind ErrorKind
}

func (err *ProviderError) Error() string {
	kind := ErrorPermanent
	if err != nil && err.Kind != "" {
		kind = err.Kind
	}
	return fmt.Sprintf("notification channel adapter %s failure", kind)
}

// ChannelAdapter is the seam between the outbox and a real provider sender.
// notification/dispatch implements it for email and SMS. Tests in this
// package use a fake.
type ChannelAdapter interface {
	Send(ctx context.Context, cmd Command) (Result, error)
}

// ErrorReasoner is an optional extension of an adapter error: a bounded,
// more specific label (e.g. "opted_out") recorded as last_error_kind.
// Never payload or provider text. Empty means "use ProviderError.Kind".
type ErrorReasoner interface{ ErrorReason() string }
