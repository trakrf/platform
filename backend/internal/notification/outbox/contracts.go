// Package outbox is the durable, provider-neutral notification delivery
// outbox (TRA-1192). It owns the transactional enqueue contract and the
// generic worker; it does NOT call any real provider. A future ticket wires
// ChannelAdapter to email.Sender (TRA-1267) / sms.Sender (TRA-1201).
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
	// Payload is opaque to the outbox: it is serialized as-is into the River
	// job args and handed unchanged to ChannelAdapter.Send. The outbox never
	// inspects it, so it never needs to know about email vs. SMS shapes.
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
// A future ticket implements this for email (wrapping email.Sender) and SMS
// (wrapping sms.Sender). Tests in this package use a fake.
type ChannelAdapter interface {
	Send(ctx context.Context, cmd Command) (Result, error)
}
