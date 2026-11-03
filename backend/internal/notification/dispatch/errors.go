// Package dispatch is the outbox.ChannelAdapter that sends a stored delivery
// through the email (Resend) or SMS (Twilio) sender.
package dispatch

import (
	"errors"
	"fmt"

	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/sms"
)

// Adapter-own failure reasons recorded as last_error_kind.
const (
	reasonInvalidPayload  = "invalid_payload"
	reasonChannelMismatch = "channel_mismatch"
	reasonDisabled        = "disabled"
	reasonOptedOut        = "opted_out"
)

// Error is every failure Send returns from mapping or its own checks. It
// carries only bounded classification, never an address, body or provider text.
type Error struct {
	Kind   outbox.ErrorKind // decides retry
	Reason string           // last_error_kind label; "" for retryable
}

func (e *Error) Error() string {
	label := string(outbox.ErrorPermanent)
	if e != nil {
		switch {
		case e.Reason != "":
			label = e.Reason
		case e.Kind != "":
			label = string(e.Kind)
		}
	}
	return fmt.Sprintf("notification delivery %s failure", label)
}

// ErrorReason is the last_error_kind label to record, or "" to use the kind.
func (e *Error) ErrorReason() string {
	if e == nil {
		return ""
	}
	return e.Reason
}

// Unwrap lets outbox.ClassifyOutcome decide retries from Kind.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return &outbox.ProviderError{Kind: e.Kind}
}

func permanent(reason string) *Error {
	return &Error{Kind: outbox.ErrorPermanent, Reason: reason}
}

func transient() *Error { return &Error{Kind: outbox.ErrorTransient} }

// mapEmailError must be called with a non-nil error.
func mapEmailError(err error) *Error {
	var pe *email.ProviderError
	if !errors.As(err, &pe) || pe == nil {
		return transient()
	}
	if pe.OutcomeUnknown {
		return &Error{Kind: outbox.ErrorOutcomeUnknown}
	}
	switch pe.Kind {
	case email.ErrorInvalid, email.ErrorDisabled, email.ErrorPermanent:
		return permanent(string(pe.Kind))
	default:
		return transient()
	}
}

// mapSMSError must be called with a non-nil error.
func mapSMSError(err error) *Error {
	var pe *sms.ProviderError
	if !errors.As(err, &pe) || pe == nil {
		return transient()
	}
	switch pe.Kind {
	case sms.ErrorPermanent, sms.ErrorRejected:
		return permanent(string(pe.Kind))
	default:
		return transient()
	}
}
