package twilio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	twilioclient "github.com/twilio/twilio-go/client"
)

// HTTP status must survive an error body the SDK cannot decode or that
// contradicts the actual response; otherwise outages become permanent failures.
func TestSendSMS_PreservesHTTPFailureStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		kind   sms.ErrorKind
		code   string
	}{
		{"html 503", 503, "<html>Unavailable</html>", sms.ErrorTransient, ""},
		{"empty 503", 503, "", sms.ErrorTransient, ""},
		{"missing JSON status", 503, `{"code":20500}`, sms.ErrorTransient, "20500"},
		{"conflicting JSON status", 503, `{"status":400,"code":20500}`, sms.ErrorTransient, "20500"},
		{"conflicting permanent code", 503, `{"status":400,"code":21610}`, sms.ErrorTransient, "21610"},
		{"plain 429", 429, "Too many requests", sms.ErrorTransient, ""},
		{"plain 403", 403, "Forbidden", sms.ErrorPermanent, ""},
		{"opted out", 400, `{"code":21610}`, sms.ErrorPermanent, "21610"},
		{"filtered", 400, `{"code":30007}`, sms.ErrorRejected, "30007"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			sender := newSender(completeSenderConfig(), &http.Client{
				Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					return httpJSONResponse(r, tc.status, tc.body), nil
				}),
			}, nil)
			submission, err := sender.SendSMS(context.Background(), sms.Command{
				ToE164: "+15555550123", Body: "test",
			})
			var providerErr *sms.ProviderError
			require.ErrorAs(t, err, &providerErr)
			require.Equal(t, sms.Submission{}, submission)
			require.Equal(t, sms.ProviderError{
				Kind: tc.kind, Code: tc.code, HTTPStatus: tc.status,
			}, *providerErr)
			require.Equal(t, 1, calls)
		})
	}
}

// A provider outage must not leave unread, unclosed bodies accumulating or
// copy private provider text into errors and observability.
func TestSendSMS_BoundsAndClosesFailedResponses(t *testing.T) {
	for _, body := range []string{
		`{"code":20500,"message":"` + sensitiveBody + `"}`,
		strings.Repeat(" ", 128<<10) + sensitiveCredential,
	} {
		t.Run(body[:10], func(t *testing.T) {
			responseBody := &trackedResponseBody{Reader: strings.NewReader(body)}
			registry := prometheus.NewRegistry()
			metrics, err := NewMetrics(registry)
			require.NoError(t, err)
			original := &http.Client{Timeout: time.Second, Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				response := httpJSONResponse(r, 503, "")
				response.Body = responseBody
				return response, nil
			})}
			sender := newSender(completeSenderConfig(), original, metrics)
			_, err = sender.SendSMS(context.Background(), sms.Command{ToE164: sensitiveDestination, Body: sensitiveBody})
			require.Error(t, err)
			require.True(t, responseBody.closed)
			require.LessOrEqual(t, responseBody.read, 64<<10)
			require.Equal(t, time.Second, original.Timeout)
			_, unchanged := original.Transport.(roundTripperFunc)
			require.True(t, unchanged)
			for _, private := range []string{sensitiveBody, sensitiveDestination, sensitiveCredential} {
				require.NotContains(t, err.Error(), private)
			}
			var restErr *twilioclient.TwilioRestError
			var transportErr *url.Error
			require.False(t, errors.As(err, &restErr))
			require.False(t, errors.As(err, &transportErr))
			families, err := registry.Gather()
			require.NoError(t, err)
			for _, family := range families {
				switch family.GetName() {
				case "trakrf_twilio_submissions_total":
					require.Len(t, family.Metric, 1)
					require.Equal(t, "transient", family.Metric[0].Label[0].GetValue())
					require.Equal(t, float64(1), family.Metric[0].Counter.GetValue())
				case "trakrf_twilio_request_duration_seconds":
					require.Equal(t, uint64(1), family.Metric[0].Histogram.GetSampleCount())
				}
			}
		})
	}
}

type trackedResponseBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *trackedResponseBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}

func (b *trackedResponseBody) Close() error {
	b.closed = true
	return nil
}
