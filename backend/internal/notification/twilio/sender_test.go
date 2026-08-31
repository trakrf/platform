package twilio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

// This fails if the public constructors lose the no-recorder default, accept a
// nil recorder incorrectly, or fail to retain an explicitly supplied recorder.
func TestNewSender_MetricsConstruction(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	require.NoError(t, err)

	tests := []struct {
		name  string
		build func() (*Sender, error)
		want  *Metrics
	}{
		{
			name: "no recorder",
			build: func() (*Sender, error) {
				return NewSender(completeSenderConfig())
			},
		},
		{
			name: "nil recorder",
			build: func() (*Sender, error) {
				return NewSenderWithMetrics(completeSenderConfig(), nil)
			},
		},
		{
			name: "one recorder",
			build: func() (*Sender, error) {
				return NewSenderWithMetrics(completeSenderConfig(), metrics)
			},
			want: metrics,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender, err := test.build()

			require.NoError(t, err)
			require.NotNil(t, sender)
			require.Same(t, test.want, sender.metrics)
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
	var normalized *providerError
	require.ErrorAs(t, err, &normalized)
	require.Equal(t, sms.ProviderError{Kind: sms.ErrorPermanent, Code: "21211", HTTPStatus: 400}, normalized.ProviderError)
	for _, sensitive := range []string{command.ToE164, command.Body, sensitiveCredential} {
		require.NotContains(t, err.Error(), sensitive)
	}
}

// This fails if an SDK response without a message is treated as success or exposes provider data.
func TestSendSMS_NormalizesNilSDKResponse(t *testing.T) {
	sender := newSenderWithMessages(completeSenderConfig(), nilResponseCreator{}, nil)

	submission, err := sender.SendSMS(context.Background(), sms.Command{ToE164: sensitiveDestination, Body: sensitiveBody})

	require.Equal(t, sms.Submission{}, submission)
	var normalized *providerError
	require.ErrorAs(t, err, &normalized)
	require.Equal(t, sms.ProviderError{Kind: sms.ErrorPermanent, Code: unknownCode}, normalized.ProviderError)
	for _, sensitive := range []string{sensitiveDestination, sensitiveBody} {
		require.NotContains(t, err.Error(), sensitive)
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

// This fails if sender outcome metrics do not represent each externally
// observable submission result once, leak request data into labels, or time a
// call that was cancelled before the SDK could make its HTTP request.
func TestSendSMS_RecordsMetricsAtProviderBoundary(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	require.NoError(t, err)

	var requests atomic.Int32
	sender := newSender(completeSenderConfig(), &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		encoded, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		form, err := url.ParseQuery(string(encoded))
		require.NoError(t, err)

		switch form.Get("To") {
		case "+15555550101":
			return httpJSONResponse(request, http.StatusCreated, `{"sid":"SMaccepted","status":"queued"}`), nil
		case "+15555550102":
			return httpJSONResponse(request, http.StatusInternalServerError, `{"code":20500,"status":500,"message":"body=transient body secret=credential"}`), nil
		case "+15555550103":
			return httpJSONResponse(request, http.StatusBadRequest, `{"code":21211,"status":400,"message":"destination=+15555550103 body=permanent body"}`), nil
		case "+15555550104":
			return httpJSONResponse(request, http.StatusBadRequest, `{"code":30007,"status":400,"message":"destination=+15555550104 body=rejected body"}`), nil
		default:
			t.Fatalf("unexpected destination %q", form.Get("To"))
			return nil, nil
		}
	})}, metrics)

	tests := []struct {
		name     string
		command  sms.Command
		wantKind sms.ErrorKind
	}{
		{
			name:    "accepted",
			command: sms.Command{ToE164: "+15555550101", Body: "accepted body"},
		},
		{
			name:     "transient provider failure",
			command:  sms.Command{ToE164: "+15555550102", Body: "transient body"},
			wantKind: sms.ErrorTransient,
		},
		{
			name:     "permanent provider failure",
			command:  sms.Command{ToE164: "+15555550103", Body: "permanent body"},
			wantKind: sms.ErrorPermanent,
		},
		{
			name:     "rejected provider failure",
			command:  sms.Command{ToE164: "+15555550104", Body: "rejected body"},
			wantKind: sms.ErrorRejected,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			submission, err := sender.SendSMS(context.Background(), test.command)

			if test.wantKind == "" {
				require.NoError(t, err)
				require.Equal(t, sms.Submission{ProviderMessageID: "SMaccepted", Status: "queued"}, submission)
				return
			}

			require.Equal(t, sms.Submission{}, submission)
			var providerErr *providerError
			require.ErrorAs(t, err, &providerErr)
			require.Equal(t, test.wantKind, providerErr.Kind)
		})
	}

	cancellation := errors.New("caller cancelled sensitive submission")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cancellation)
	submission, err := sender.SendSMS(ctx, sms.Command{ToE164: "+15555550105", Body: "cancelled body"})
	require.Equal(t, sms.Submission{}, submission)
	require.ErrorIs(t, err, cancellation)
	require.Equal(t, int32(4), requests.Load())

	families, err := registry.Gather()
	require.NoError(t, err)
	submissions := map[string]float64{}
	var durationSamples uint64
	for _, family := range families {
		switch family.GetName() {
		case "trakrf_twilio_submissions_total":
			for _, metric := range family.GetMetric() {
				require.Len(t, metric.GetLabel(), 1)
				label := metric.GetLabel()[0]
				require.Equal(t, "result", label.GetName())
				submissions[label.GetValue()] = metric.GetCounter().GetValue()
			}
		case "trakrf_twilio_request_duration_seconds":
			require.Len(t, family.GetMetric(), 1)
			require.Empty(t, family.GetMetric()[0].GetLabel())
			durationSamples = family.GetMetric()[0].GetHistogram().GetSampleCount()
		case "trakrf_twilio_callbacks_total":
			t.Fatalf("sender submission created callback metric series")
		default:
			t.Fatalf("unexpected metric family %q", family.GetName())
		}
	}

	require.Equal(t, map[string]float64{
		"accepted":  1,
		"transient": 1,
		"permanent": 1,
		"rejected":  1,
		"unknown":   1,
	}, submissions)
	require.Equal(t, uint64(4), durationSamples)
	for _, label := range []string{"+15555550101", "+15555550102", "+15555550103", "+15555550104", "accepted body", "transient body", "permanent body", "rejected body", "cancelled body", "SMaccepted", sensitiveCredential, testAPIKeySecret} {
		require.NotContains(t, submissions, label)
	}
}

// This fails if an attempted send with no SDK message or an unrecognised
// transport failure is exported as a known provider outcome instead of the
// bounded unknown metric result.
func TestSendSMS_RecordsUnknownMetricsForUnexpectedOutcomes(t *testing.T) {
	tests := []struct {
		name  string
		build func(*Metrics) *Sender
	}{
		{
			name: "unrecognised transport failure",
			build: func(metrics *Metrics) *Sender {
				return newSender(completeSenderConfig(), &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
					return nil, errors.New("unrecognised transport failure")
				})}, metrics)
			},
		},
		{
			name: "nil SDK response",
			build: func(metrics *Metrics) *Sender {
				return newSenderWithMessages(completeSenderConfig(), nilResponseCreator{}, metrics)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			metrics, err := NewMetrics(registry)
			require.NoError(t, err)
			sender := test.build(metrics)

			submission, err := sender.SendSMS(context.Background(), sms.Command{ToE164: sensitiveDestination, Body: sensitiveBody})

			require.Equal(t, sms.Submission{}, submission)
			var providerErr *providerError
			require.ErrorAs(t, err, &providerErr)
			require.Equal(t, sms.ProviderError{Kind: sms.ErrorPermanent, Code: unknownCode}, providerErr.ProviderError)

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
					require.Equal(t, "result", metric.GetLabel()[0].GetName())
					require.Equal(t, metricSubmissionResultUnknown, metric.GetLabel()[0].GetValue())
				case "trakrf_twilio_request_duration_seconds":
					sawDuration = true
					require.Len(t, family.GetMetric(), 1)
					require.Equal(t, uint64(1), family.GetMetric()[0].GetHistogram().GetSampleCount())
				case "trakrf_twilio_callbacks_total":
					t.Fatalf("sender submission created callback metric series")
				default:
					t.Fatalf("unexpected metric family %q", family.GetName())
				}
			}
			require.True(t, sawSubmission)
			require.True(t, sawDuration)
		})
	}
}

// This fails if concurrent sends share mutable SDK request state on the production HTTP path.
func TestSendSMS_IsSafeForConcurrentUse(t *testing.T) {
	metrics, err := NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	sender := newSender(completeSenderConfig(), &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return httpJSONResponse(request, http.StatusCreated, `{"sid":"SM123","status":"queued"}`), nil
	})}, metrics)

	const workers = 32
	start := make(chan struct{})
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for i := range workers {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			<-start
			submission, err := sender.SendSMS(context.Background(), sms.Command{
				ToE164: fmt.Sprintf("+15555550%03d", i),
				Body:   "ready",
			})
			if err != nil {
				errs <- err
				return
			}
			if submission != (sms.Submission{ProviderMessageID: "SM123", Status: "queued"}) {
				errs <- fmt.Errorf("submission = %#v", submission)
			}
		}(i)
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
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
