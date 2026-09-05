package twilio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/twilio/twilio-go/rest/api/v2010"
)

var _ sms.Sender = (*Sender)(nil)
var _ func(Config) (*Sender, error) = NewSender

const (
	testAccountSID          = "AC1234567890"
	testAPIKeySID           = "SK1234567890"
	testAPIKeySecret        = "apikeysecretvalue"
	testAuthToken           = "auth-token-secret-value"
	testMessagingServiceSID = "MG1234567890"
	testPublicBaseURL       = "https://api.example.com"
)

// This fails if a disabled or malformed configuration can create an outbound sender.
func TestNewSender_RejectsDisabledAndInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "disabled"},
		{
			name: "invalid callback origin",
			config: Config{
				AccountSID:          testAccountSID,
				APIKeySID:           testAPIKeySID,
				APIKeySecret:        testAPIKeySecret,
				AuthToken:           testAuthToken,
				MessagingServiceSID: testMessagingServiceSID,
				PublicBaseURL:       "https://api.example.com/",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender, err := NewSender(test.config)

			require.Error(t, err)
			require.Nil(t, sender)
			require.NotContains(t, err.Error(), testAPIKeySecret)
			require.NotContains(t, err.Error(), testAuthToken)
		})
	}
}

// This fails if the sender emits anything but Twilio's API-key-authenticated
// Messages request or returns a response SID and status incorrectly.
func TestSendSMS_SubmitsTwilioMessagesRequest(t *testing.T) {
	command := sms.Command{ToE164: "+15555550123", Body: "Your tracker is ready."}
	sender := newSender(completeSenderConfig(), &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "/2010-04-01/Accounts/"+testAccountSID+"/Messages.json", request.URL.Path)
		username, password, ok := request.BasicAuth()
		require.True(t, ok)
		require.Equal(t, testAPIKeySID, username)
		require.Equal(t, testAPIKeySecret, password)
		require.Equal(t, "application/x-www-form-urlencoded", request.Header.Get("Content-Type"))

		encoded, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		form, err := url.ParseQuery(string(encoded))
		require.NoError(t, err)
		require.Equal(t, url.Values{
			"To":                  {command.ToE164},
			"Body":                {command.Body},
			"MessagingServiceSid": {testMessagingServiceSID},
			"StatusCallback":      {testPublicBaseURL + statusCallbackPath},
		}, form)

		return httpJSONResponse(request, http.StatusCreated, `{"sid":"SM123","status":"queued"}`), nil
	})}, nil)

	submission, err := sender.SendSMS(context.Background(), command)

	require.NoError(t, err)
	require.Equal(t, sms.Submission{ProviderMessageID: "SM123", Status: "queued"}, submission)
}

// This fails if HTTP provider failures bypass the normalized redacted SMS error contract.
func TestSendSMS_NormalizesAndRedactsProviderHTTPError(t *testing.T) {
	command := sms.Command{ToE164: sensitiveDestination, Body: sensitiveBody}
	sender := newSender(completeSenderConfig(), &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return httpJSONResponse(request, http.StatusBadRequest, fmt.Sprintf(`{"code":21211,"status":400,"message":"destination=%s body=%s secret=%s"}`, command.ToE164, command.Body, sensitiveCredential)), nil
	})}, nil)

	submission, err := sender.SendSMS(context.Background(), command)

	require.Equal(t, sms.Submission{}, submission)
	var normalized *sms.ProviderError
	require.ErrorAs(t, err, &normalized)
	require.Equal(t, sms.ProviderError{Kind: sms.ErrorPermanent, Code: "21211", HTTPStatus: 400}, *normalized)
	for _, sensitive := range []string{command.ToE164, command.Body, sensitiveCredential} {
		require.NotContains(t, err.Error(), sensitive)
	}
}

// This fails if an SDK response without a message is treated as success or exposes provider data.
func TestSendSMS_NormalizesNilSDKResponse(t *testing.T) {
	sender := newSenderWithMessages(completeSenderConfig(), nilResponseCreator{}, nil)

	submission, err := sender.SendSMS(context.Background(), sms.Command{ToE164: sensitiveDestination, Body: sensitiveBody})

	require.Equal(t, sms.Submission{}, submission)
	var normalized *sms.ProviderError
	require.ErrorAs(t, err, &normalized)
	require.Equal(t, sms.ProviderError{Kind: sms.ErrorPermanent, Code: unknownCode}, *normalized)
	for _, sensitive := range []string{sensitiveDestination, sensitiveBody} {
		require.NotContains(t, err.Error(), sensitive)
	}
}

// This fails if a 2xx SDK response without both an accepted Message SID and
// initial status becomes an unusable accepted submission or known outcome.
func TestSendSMS_NormalizesIncompleteAcceptedSDKResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty object", body: `{}`},
		{name: "missing message SID", body: `{"status":"queued"}`},
		{name: "empty message SID", body: `{"sid":"","status":"queued"}`},
		{name: "whitespace message SID", body: `{"sid":" ","status":"queued"}`},
		{name: "missing initial status", body: `{"sid":"SM123"}`},
		{name: "empty initial status", body: `{"sid":"SM123","status":""}`},
		{name: "whitespace initial status", body: `{"sid":"SM123","status":" "}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			metrics, err := NewMetrics(registry)
			require.NoError(t, err)

			var requests atomic.Int32
			sender := newSender(completeSenderConfig(), &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
				requests.Add(1)
				return httpJSONResponse(request, http.StatusCreated, test.body), nil
			})}, metrics)

			submission, err := sender.SendSMS(context.Background(), sms.Command{ToE164: sensitiveDestination, Body: sensitiveBody})

			require.Equal(t, sms.Submission{}, submission)
			var providerErr *sms.ProviderError
			require.ErrorAs(t, err, &providerErr)
			require.Equal(t, sms.ProviderError{Kind: sms.ErrorPermanent, Code: unknownCode}, *providerErr)
			require.Equal(t, int32(1), requests.Load())
			for _, sensitive := range []string{sensitiveDestination, sensitiveBody, testAPIKeySecret} {
				require.NotContains(t, err.Error(), sensitive)
			}

			families, err := registry.Gather()
			require.NoError(t, err)
			var sawSubmission, sawDuration bool
			for _, family := range families {
				switch family.GetName() {
				case "trakrf_twilio_submissions_total":
					sawSubmission = true
					require.Len(t, family.GetMetric(), 1)
					metric := family.GetMetric()[0]
					require.Equal(t, float64(1), metric.GetCounter().GetValue())
					require.Len(t, metric.GetLabel(), 1)
					require.Equal(t, metricSubmissionResultUnknown, metric.GetLabel()[0].GetValue())
				case "trakrf_twilio_request_duration_seconds":
					sawDuration = true
					require.Len(t, family.GetMetric(), 1)
					require.Equal(t, uint64(1), family.GetMetric()[0].GetHistogram().GetSampleCount())
				default:
					t.Fatalf("unexpected metric family %q", family.GetName())
				}
			}
			require.True(t, sawSubmission)
			require.True(t, sawDuration)
		})
	}
}

// This fails if a caller-cancelled context reaches Twilio or is recast as a provider failure.
func TestSendSMS_ReturnsCallerCancellationWithoutOutboundRequest(t *testing.T) {
	var requests atomic.Int32
	sender := newSender(completeSenderConfig(), &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		return httpJSONResponse(request, http.StatusCreated, `{"sid":"SM123","status":"queued"}`), nil
	})}, nil)
	contextCause := errors.New("caller cancelled this submission")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(contextCause)

	submission, err := sender.SendSMS(ctx, sms.Command{ToE164: "+15555550123", Body: "ready"})

	require.Equal(t, sms.Submission{}, submission)
	require.ErrorIs(t, err, contextCause)
	require.Zero(t, requests.Load())
}

type nilResponseCreator struct{}

func (nilResponseCreator) CreateMessage(*openapi.CreateMessageParams) (*openapi.ApiV2010Message, error) {
	return nil, nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func httpJSONResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func completeSenderConfig() Config {
	return Config{
		AccountSID:          testAccountSID,
		APIKeySID:           testAPIKeySID,
		APIKeySecret:        testAPIKeySecret,
		AuthToken:           testAuthToken,
		MessagingServiceSID: testMessagingServiceSID,
		PublicBaseURL:       testPublicBaseURL,
	}
}
