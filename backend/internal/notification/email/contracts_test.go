package email_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/trakrf/platform/backend/internal/notification/email"
)

// These doubles deliberately depend only on the neutral package. Provider SDK
// types in either interface would prevent downstream workflow tests compiling.
type senderFunc func(context.Context, email.Command) (email.Submission, error)

func (f senderFunc) SendEmail(ctx context.Context, cmd email.Command) (email.Submission, error) {
	return f(ctx, cmd)
}

type consumerFunc func(context.Context, email.CallbackEvent) error

func (f consumerFunc) HandleEvent(ctx context.Context, event email.CallbackEvent) error {
	return f(ctx, event)
}

var _ email.Sender = senderFunc(nil)
var _ email.CallbackConsumer = consumerFunc(nil)

func TestProviderErrorIsSafe(t *testing.T) {
	for _, kind := range []email.ErrorKind{email.ErrorInvalid, email.ErrorDisabled, email.ErrorPermanent, email.ErrorTransient, email.ErrorCanceled, email.ErrorTimeout, email.ErrorUnknown, "secret@example.com"} {
		err := &email.ProviderError{Kind: kind, HTTPStatus: 503, OutcomeUnknown: true}
		if strings.Contains(err.Error(), "secret") {
			t.Fatal("untrusted error category leaked")
		}
		var normalized *email.ProviderError
		if !errors.As(err, &normalized) || !normalized.OutcomeUnknown {
			t.Fatal("lost structured ambiguity")
		}
	}
}

func TestContextErrorsRemainDiscoverable(t *testing.T) {
	if !errors.Is(&email.ProviderError{Kind: email.ErrorCanceled}, context.Canceled) {
		t.Fatal("cancellation lost")
	}
	if !errors.Is(&email.ProviderError{Kind: email.ErrorTimeout}, context.DeadlineExceeded) {
		t.Fatal("deadline lost")
	}
	if errors.Is(&email.ProviderError{Kind: email.ErrorPermanent}, context.Canceled) {
		t.Fatal("false cancellation")
	}
}
