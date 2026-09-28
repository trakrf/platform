// Package email defines provider-neutral notification email contracts. It is
// independent of the transactional email service and provider SDKs.
package email

import (
	"context"
	"fmt"
	"time"
)

// Command submits one delivery to one recipient. DeliveryID must be stable
// across attempts and globally unique across notification deliveries. Text or
// HTML (or both) must be provided. To contains a single bare email address.
type Command struct {
	DeliveryID string
	To         string
	Subject    string
	Text       string
	HTML       string
}

// Submission means the provider accepted the request, NOT that the recipient
// received the email. Delivery is reported separately through CallbackConsumer.
// On failure the sender returns a zero Submission and a normalized error.
type Submission struct {
	ProviderMessageID string
}

type Sender interface {
	SendEmail(context.Context, Command) (Submission, error)
}

// ErrorKind is a bounded failure category, suitable for logs and metrics.
type ErrorKind string

const (
	ErrorInvalid   ErrorKind = "invalid"
	ErrorDisabled  ErrorKind = "disabled"
	ErrorPermanent ErrorKind = "permanent"
	ErrorTransient ErrorKind = "transient"
	ErrorCanceled  ErrorKind = "canceled"
	ErrorTimeout   ErrorKind = "timeout"
	ErrorUnknown   ErrorKind = "unknown"
)

// ProviderError never retains provider response text, request data or causes.
// OutcomeUnknown means the provider MAY have accepted the submission. Neither
// transient nor timeout implies it is safe to retry without idempotency.
type ProviderError struct {
	Kind           ErrorKind
	HTTPStatus     int
	OutcomeUnknown bool
}

func (err *ProviderError) Error() string {
	kind := ErrorUnknown
	if err != nil {
		switch err.Kind {
		case ErrorInvalid, ErrorDisabled, ErrorPermanent, ErrorTransient, ErrorCanceled, ErrorTimeout:
			kind = err.Kind
		}
	}
	return fmt.Sprintf("email provider %s failure", kind)
}

// Is preserves standard context matching without exposing the original error.
func (err *ProviderError) Is(target error) bool {
	return err != nil && ((err.Kind == ErrorCanceled && target == context.Canceled) ||
		(err.Kind == ErrorTimeout && target == context.DeadlineExceeded))
}

type EventType string

const (
	EventDelivered  EventType = "delivered"
	EventFailed     EventType = "failed"
	EventBounced    EventType = "bounced"
	EventComplained EventType = "complained"
)

// CallbackEvent is an immutable normalized provider event. Provider together
// with ProviderEventID is its deduplication identity, NOT ProviderMessageID:
// several events can refer to one message. OccurredAt is provider time;
// ReceivedAt is local receipt time. Events can arrive out of order and before
// any outgoing record exists. No recipient, body or raw failure text is kept.
type CallbackEvent struct {
	Provider          string
	ProviderEventID   string
	ProviderMessageID string
	Type              EventType
	OccurredAt        time.Time
	ReceivedAt        time.Time
}

// CallbackConsumer durably records an event before returning nil. A duplicate
// is also success. Errors mean the webhook should receive a retryable response.
type CallbackConsumer interface {
	HandleEvent(context.Context, CallbackEvent) error
}
