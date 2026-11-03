package dispatch

import (
	"errors"
	"fmt"
	"testing"

	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/outbox"
	"github.com/trakrf/platform/backend/internal/notification/sms"
)

func TestMapEmailError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		kind   outbox.ErrorKind
		reason string
	}{
		{"outcome unknown wins", &email.ProviderError{Kind: email.ErrorTimeout, OutcomeUnknown: true}, outbox.ErrorOutcomeUnknown, ""},
		{"outcome unknown on permanent", &email.ProviderError{Kind: email.ErrorPermanent, OutcomeUnknown: true}, outbox.ErrorOutcomeUnknown, ""},
		{"invalid", &email.ProviderError{Kind: email.ErrorInvalid}, outbox.ErrorPermanent, "invalid"},
		{"disabled", &email.ProviderError{Kind: email.ErrorDisabled}, outbox.ErrorPermanent, "disabled"},
		{"permanent", &email.ProviderError{Kind: email.ErrorPermanent}, outbox.ErrorPermanent, "permanent"},
		{"transient", &email.ProviderError{Kind: email.ErrorTransient}, outbox.ErrorTransient, ""},
		{"timeout", &email.ProviderError{Kind: email.ErrorTimeout}, outbox.ErrorTransient, ""},
		{"canceled", &email.ProviderError{Kind: email.ErrorCanceled}, outbox.ErrorTransient, ""},
		{"unknown", &email.ProviderError{Kind: email.ErrorUnknown}, outbox.ErrorTransient, ""},
		{"raw", errors.New("x"), outbox.ErrorTransient, ""},
		{"typed nil", (*email.ProviderError)(nil), outbox.ErrorTransient, ""},
		{"wrapped invalid", fmt.Errorf("x: %w", &email.ProviderError{Kind: email.ErrorInvalid}), outbox.ErrorPermanent, "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapEmailError(tt.err)
			if got.Kind != tt.kind || got.Reason != tt.reason {
				t.Fatalf("got {%q %q}, want {%q %q}", got.Kind, got.Reason, tt.kind, tt.reason)
			}
		})
	}
}

func TestMapSMSError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		kind   outbox.ErrorKind
		reason string
	}{
		{"permanent", &sms.ProviderError{Kind: sms.ErrorPermanent}, outbox.ErrorPermanent, "permanent"},
		{"rejected", &sms.ProviderError{Kind: sms.ErrorRejected}, outbox.ErrorPermanent, "rejected"},
		{"transient", &sms.ProviderError{Kind: sms.ErrorTransient}, outbox.ErrorTransient, ""},
		{"other kind", &sms.ProviderError{Kind: "weird"}, outbox.ErrorTransient, ""},
		{"raw", errors.New("x"), outbox.ErrorTransient, ""},
		{"typed nil", (*sms.ProviderError)(nil), outbox.ErrorTransient, ""},
		{"wrapped rejected", fmt.Errorf("x: %w", &sms.ProviderError{Kind: sms.ErrorRejected}), outbox.ErrorPermanent, "rejected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapSMSError(tt.err)
			if got.Kind != tt.kind || got.Reason != tt.reason {
				t.Fatalf("got {%q %q}, want {%q %q}", got.Kind, got.Reason, tt.kind, tt.reason)
			}
		})
	}
}

func TestError_UnwrapsToProviderErrorForClassify(t *testing.T) {
	e := &Error{Kind: outbox.ErrorPermanent, Reason: "opted_out"}
	if got := outbox.ClassifyOutcome(e); got != outbox.DeliveryPermanentFailure {
		t.Fatalf("ClassifyOutcome = %v, want permanent failure", got)
	}
	if e.ErrorReason() != "opted_out" {
		t.Fatalf("ErrorReason = %q", e.ErrorReason())
	}
}
