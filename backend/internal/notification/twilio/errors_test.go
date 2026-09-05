package twilio

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	twilioclient "github.com/twilio/twilio-go/client"
)

const (
	sensitiveDestination = "+15555550123"
	sensitiveBody        = "verification code 654321"
	sensitiveCredential  = "api-key-secret-value"
)

// This fails if Twilio and network failures are classified into an incorrect
// stable provider error, or if classification follows raw provider text.
func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want sms.ProviderError
	}{
		{
			name: "permanent invalid destination",
			err:  twilioRESTError(21211, 400),
			want: sms.ProviderError{Kind: sms.ErrorPermanent, Code: "21211", HTTPStatus: 400},
		},
		{
			name: "permanent permission failure",
			err:  twilioRESTError(21408, 403),
			want: sms.ProviderError{Kind: sms.ErrorPermanent, Code: "21408", HTTPStatus: 403},
		},
		{
			name: "permanent unverified destination",
			err:  twilioRESTError(21610, 400),
			want: sms.ProviderError{Kind: sms.ErrorPermanent, Code: "21610", HTTPStatus: 400},
		},
		{
			name: "permanent invalid destination format",
			err:  twilioRESTError(21612, 400),
			want: sms.ProviderError{Kind: sms.ErrorPermanent, Code: "21612", HTTPStatus: 400},
		},
		{
			name: "permanent queue overflow",
			err:  twilioRESTError(30034, 400),
			want: sms.ProviderError{Kind: sms.ErrorPermanent, Code: "30034", HTTPStatus: 400},
		},
		{
			name: "rejected carrier violation",
			err:  twilioRESTError(30007, 400),
			want: sms.ProviderError{Kind: sms.ErrorRejected, Code: "30007", HTTPStatus: 400},
		},
		{
			name: "rejected opt out",
			err:  &twilioclient.RestErrorV1{Code: 30450, HttpStatusCode: 400, Message: sensitiveBody, Params: sensitiveParams()},
			want: sms.ProviderError{Kind: sms.ErrorRejected, Code: "30450", HTTPStatus: 400},
		},
		{
			name: "permanent unknown client error",
			err:  fmt.Errorf("provider request: %w", twilioRESTError(20999, 404)),
			want: sms.ProviderError{Kind: sms.ErrorPermanent, Code: "20999", HTTPStatus: 404},
		},
		{
			name: "transient rate limit",
			err:  twilioRESTError(20429, 429),
			want: sms.ProviderError{Kind: sms.ErrorTransient, Code: "20429", HTTPStatus: 429},
		},
		{
			name: "transient server error",
			err:  twilioRESTError(0, 503),
			want: sms.ProviderError{Kind: sms.ErrorTransient, Code: "", HTTPStatus: 503},
		},
		{
			name: "permanent invalid non HTTP status",
			err:  twilioRESTError(0, 600),
			want: sms.ProviderError{Kind: sms.ErrorPermanent, Code: "", HTTPStatus: 600},
		},
		{
			name: "transient timeout",
			err:  &net.DNSError{Name: sensitiveDestination, Server: sensitiveCredential, IsTimeout: true},
			want: sms.ProviderError{Kind: sms.ErrorTransient, Code: "timeout"},
		},
		{
			name: "transient temporary network failure",
			err:  &net.DNSError{Name: sensitiveDestination, Server: sensitiveCredential, IsTemporary: true},
			want: sms.ProviderError{Kind: sms.ErrorTransient, Code: "temporary_network"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyError(test.err)

			var normalized *sms.ProviderError
			require.ErrorAs(t, got, &normalized)
			require.Equal(t, test.want, *normalized)
			for _, sensitive := range []string{sensitiveDestination, sensitiveBody, sensitiveCredential} {
				require.NotContains(t, got.Error(), sensitive)
			}
		})
	}
}

// This fails if a normalized error keeps the raw Twilio error, whose message
// and details may contain request data, in its error chain.
func TestClassifyError_RedactsProviderErrorChain(t *testing.T) {
	raw := twilioRESTError(21211, 400)

	classified := classifyError(raw)

	var exposed *twilioclient.TwilioRestError
	require.False(t, errors.As(classified, &exposed))
}

func twilioRESTError(code, status int) *twilioclient.TwilioRestError {
	return &twilioclient.TwilioRestError{
		Code:    code,
		Status:  status,
		Message: sensitiveBody,
		Details: sensitiveParams(),
	}
}

func sensitiveParams() map[string]interface{} {
	return map[string]interface{}{
		"to":          sensitiveDestination,
		"body":        sensitiveBody,
		"credentials": sensitiveCredential,
	}
}
