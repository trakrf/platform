package resend

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/email"
)

func TestDeliveryIdempotencyThroughSDK(t *testing.T) {
	var keys []string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		return response(200, `{"id":"original-message"}`), nil
	})
	cmd := command()
	for range 2 {
		// Fresh instances must derive the same key after a process restart.
		result, err := newTestSender(t, transport).SendEmail(context.Background(), cmd)
		require.NoError(t, err)
		require.Equal(t, "original-message", result.ProviderMessageID)
	}
	cmd.Subject, cmd.Text, cmd.HTML, cmd.To = "Changed", "Changed", "<p>Changed</p>", "other@example.com"
	_, err := newTestSender(t, transport).SendEmail(context.Background(), cmd)
	require.NoError(t, err)
	cmd.DeliveryID = strings.Repeat("delivery/\r\n世界", 100)
	_, err = newTestSender(t, transport).SendEmail(context.Background(), cmd)
	require.NoError(t, err)
	require.NotEmpty(t, keys[0])
	require.Equal(t, keys[0], keys[1])
	require.Equal(t, keys[0], keys[2], "payload must not change identity")
	require.NotEqual(t, keys[0], keys[3])
	for _, key := range keys {
		require.LessOrEqual(t, len(key), 256)
		require.Regexp(t, `^notification-email/v1/[a-f0-9]{64}$`, key)
	}
}

func TestAmbiguousSubmissionKeepsDeliveryKey(t *testing.T) {
	for _, mode := range []string{"lost response", "server error", "invalid success"} {
		t.Run(mode, func(t *testing.T) {
			var keys []string
			sender := newTestSender(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
				keys = append(keys, r.Header.Get("Idempotency-Key"))
				if len(keys) == 1 {
					switch mode {
					case "lost response":
						return nil, errors.New("connection lost")
					case "server error":
						return response(503, ""), nil
					default:
						return response(200, `{}`), nil
					}
				}
				return response(200, `{"id":"accepted-before-response-loss"}`), nil
			}))
			_, err := sender.SendEmail(context.Background(), command())
			var failure *email.ProviderError
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.OutcomeUnknown)
			require.Len(t, keys, 1, "adapter must not retry")
			result, err := sender.SendEmail(context.Background(), command())
			require.NoError(t, err)
			require.Equal(t, "accepted-before-response-loss", result.ProviderMessageID)
			require.Equal(t, keys[0], keys[1])
		})
	}
}

func TestIdempotencyConflicts(t *testing.T) {
	for _, tc := range []struct {
		body    string
		kind    email.ErrorKind
		unknown bool
	}{
		{`{"name":"invalid_idempotent_request","message":"private recipient"}`, email.ErrorPermanent, false},
		{`{"name":"concurrent_idempotent_requests","message":"private recipient"}`, email.ErrorTransient, true},
		{`{"name":"future_conflict"}`, email.ErrorPermanent, true},
		{`not json`, email.ErrorPermanent, true},
		{`{"name":"concurrent_idempotent_requests","message":"` + strings.Repeat("x", 8192) + `"}`, email.ErrorPermanent, true},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			calls := 0
			sender := newTestSender(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return response(409, tc.body), nil
			}))
			_, err := sender.SendEmail(context.Background(), command())
			require.Equal(t, &email.ProviderError{Kind: tc.kind, HTTPStatus: 409, OutcomeUnknown: tc.unknown}, err)
			require.NotContains(t, err.Error(), "private")
			require.Nil(t, errors.Unwrap(err))
			require.Equal(t, 1, calls)
		})
	}
}
