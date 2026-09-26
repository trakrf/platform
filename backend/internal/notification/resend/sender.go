package resend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	sdk "github.com/resend/resend-go/v2"
	"github.com/trakrf/platform/backend/internal/notification/email"
)

// Sender submits notification email through the Resend SDK. It is safe for
// concurrent use and never retries. Construction does not send traffic.
type Sender struct {
	client  *sdk.Client
	from    string
	timeout time.Duration
}

var _ email.Sender = (*Sender)(nil)

func NewSender(config Config) (*Sender, error) {
	return NewSenderWithHTTPClient(config, nil)
}

// NewSenderWithHTTPClient allows transport injection without replacing the SDK.
// The supplied client is copied; redirects are always disabled for submissions.
func NewSenderWithHTTPClient(config Config, original *http.Client) (*Sender, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, &email.ProviderError{Kind: email.ErrorDisabled}
	}
	client := sdk.NewCustomClient(providerHTTPClient(original), config.APIKey)
	// Do not inherit the SDK's process-global RESEND_BASE_URL override: only the
	// injected transport is variable, so notification credentials stay at Resend.
	client.BaseURL = &url.URL{Scheme: "https", Host: "api.resend.com", Path: "/"}
	return &Sender{client: client, from: config.From, timeout: config.Timeout}, nil
}

func (s *Sender) SendEmail(ctx context.Context, cmd email.Command) (email.Submission, error) {
	if strings.TrimSpace(cmd.DeliveryID) == "" || !validAddress(cmd.To, false) ||
		strings.TrimSpace(cmd.Subject) == "" || strings.ContainsAny(cmd.Subject, "\r\n") ||
		(strings.TrimSpace(cmd.Text) == "" && strings.TrimSpace(cmd.HTML) == "") {
		return email.Submission{}, &email.ProviderError{Kind: email.ErrorInvalid}
	}
	if err := ctx.Err(); err != nil {
		return email.Submission{}, classifyError(err, false)
	}
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	sent, err := s.client.Emails.SendWithOptions(ctx, &sdk.SendEmailRequest{
		From:    s.from,
		To:      []string{cmd.To},
		Subject: cmd.Subject,
		Text:    cmd.Text,
		Html:    cmd.HTML,
	}, &sdk.SendEmailOptions{IdempotencyKey: deliveryKey(cmd.DeliveryID)})
	if err != nil {
		return email.Submission{}, classifyError(err, true)
	}
	if sent == nil || strings.TrimSpace(sent.Id) == "" {
		return email.Submission{}, &email.ProviderError{Kind: email.ErrorUnknown, OutcomeUnknown: true}
	}
	return email.Submission{ProviderMessageID: sent.Id}, nil
}

func classifyError(err error, submitted bool) *email.ProviderError {
	var failure *httpFailure
	if errors.As(err, &failure) {
		result := &email.ProviderError{Kind: email.ErrorPermanent, HTTPStatus: failure.status}
		switch {
		case failure.status == http.StatusConflict:
			// A concurrent or unrecognized conflict can refer to a submission
			// still in flight. Only the explicit payload mismatch is known.
			result.OutcomeUnknown = failure.conflict != "invalid_idempotent_request"
			if failure.conflict == "concurrent_idempotent_requests" {
				result.Kind = email.ErrorTransient
			}
		case failure.status == http.StatusTooManyRequests:
			result.Kind = email.ErrorTransient
		case failure.status == http.StatusRequestTimeout || failure.status >= 500:
			result.Kind = email.ErrorTransient
			result.OutcomeUnknown = true
		}
		return result
	}
	if errors.Is(err, context.Canceled) {
		return &email.ProviderError{Kind: email.ErrorCanceled, OutcomeUnknown: submitted}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &email.ProviderError{Kind: email.ErrorTimeout, OutcomeUnknown: submitted}
	}
	// HTTP/network errors include connection failures and lost responses. Never
	// infer rejection from absence of a response, nor retain the unsafe cause.
	var networkError *url.Error
	if errors.As(err, &networkError) {
		return &email.ProviderError{Kind: email.ErrorTransient, OutcomeUnknown: submitted}
	}
	// A successful HTTP response that the SDK cannot decode may already have sent.
	return &email.ProviderError{Kind: email.ErrorUnknown, OutcomeUnknown: submitted}
}

// deliveryKey is versioned and independent of attempts, sender configuration,
// and payload. Hashing keeps arbitrary delivery identities within header limits.
// Resend retains keys for 24 hours; this is not a permanent exactly-once promise.
// Keep the payload (including From) unchanged on retries. A payload mismatch is
// permanent: never evade it by generating a fresh key for the same delivery.
// https://resend.com/docs/dashboard/emails/idempotency-keys
func deliveryKey(deliveryID string) string {
	digest := sha256.Sum256([]byte(deliveryID))
	return "notification-email/v1/" + hex.EncodeToString(digest[:])
}
