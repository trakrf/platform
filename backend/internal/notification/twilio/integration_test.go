package twilio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
)

// This integration check joins valid sender configuration, the official SDK's
// HTTP transport path, bounded metrics, and normalized errors. The boundary
// does not emit logs, so its privacy evidence is the returned error and the
// gathered metric labels rather than a log assertion.
func TestSenderIntegration_SubmitsThroughMessagingServiceWithBoundedOutcomes(t *testing.T) {
	const (
		privateDestination = "+15555550199"
		privateBody        = "integration message body"
		privateCredential  = "integrationapikeysecret"
	)

	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	require.NoError(t, err)

	var requests atomic.Int32
	var transportErr error
	var transportErrMu sync.Mutex
	recordTransportError := func(err error) {
		transportErrMu.Lock()
		defer transportErrMu.Unlock()
		if transportErr == nil {
			transportErr = err
		}
	}

	config := Config{
		AccountSID:          "ACintegrationaccount",
		APIKeySID:           "SKintegrationkey",
		APIKeySecret:        privateCredential,
		AuthToken:           "integration-auth-token",
		MessagingServiceSID: "MG-integration-service",
		PublicBaseURL:       "https://callbacks.integration.example",
	}
	sender := newSender(config, &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/2010-04-01/Accounts/"+config.AccountSID+"/Messages.json" {
			recordTransportError(fmt.Errorf("unexpected Twilio request %s %s", request.Method, request.URL.Path))
		}

		encoded, err := io.ReadAll(request.Body)
		if err != nil {
			recordTransportError(fmt.Errorf("read request body: %w", err))
			return nil, err
		}
		form, err := url.ParseQuery(string(encoded))
		if err != nil {
			recordTransportError(fmt.Errorf("parse request form: %w", err))
			return nil, err
		}
		if got, want := form.Get("MessagingServiceSid"), config.MessagingServiceSID; got != want {
			recordTransportError(fmt.Errorf("MessagingServiceSid = %q, want %q", got, want))
		}
		if got := form.Get("From"); got != "" {
			recordTransportError(fmt.Errorf("raw From = %q", got))
		}
		if got, want := form.Get("StatusCallback"), config.PublicBaseURL+statusCallbackPath; got != want {
			recordTransportError(fmt.Errorf("StatusCallback = %q, want %q", got, want))
		}

		if form.Get("To") == privateDestination {
			return httpJSONResponse(request, http.StatusBadRequest, fmt.Sprintf(`{"code":21211,"status":400,"message":"to=%s body=%s credential=%s"}`, privateDestination, privateBody, privateCredential)), nil
		}
		return httpJSONResponse(request, http.StatusCreated, `{"sid":"SM-integration-submission","status":"queued"}`), nil
	})}, metrics)

	submission, err := sender.SendSMS(context.Background(), sms.Command{ToE164: "+15555550100", Body: "ready"})
	require.NoError(t, err)
	require.Equal(t, sms.Submission{ProviderMessageID: "SM-integration-submission", Status: "queued"}, submission)

	submission, err = sender.SendSMS(context.Background(), sms.Command{ToE164: privateDestination, Body: privateBody})
	require.Equal(t, sms.Submission{}, submission)
	var providerErr *providerError
	require.ErrorAs(t, err, &providerErr)
	require.Equal(t, sms.ProviderError{Kind: sms.ErrorPermanent, Code: "21211", HTTPStatus: http.StatusBadRequest}, providerErr.ProviderError)
	for _, sensitive := range []string{privateDestination, privateBody, privateCredential} {
		require.NotContains(t, err.Error(), sensitive)
	}

	const concurrentSends = 12
	start := make(chan struct{})
	errs := make(chan error, concurrentSends)
	var group sync.WaitGroup
	for i := range concurrentSends {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			<-start
			got, err := sender.SendSMS(context.Background(), sms.Command{
				ToE164: fmt.Sprintf("+15555552%03d", i),
				Body:   "concurrent integration send",
			})
			if err != nil {
				errs <- err
				return
			}
			if got != (sms.Submission{ProviderMessageID: "SM-integration-submission", Status: "queued"}) {
				errs <- fmt.Errorf("submission = %#v", got)
			}
		}(i)
	}
	close(start)
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	transportErrMu.Lock()
	require.NoError(t, transportErr)
	transportErrMu.Unlock()
	require.Equal(t, int32(concurrentSends+2), requests.Load())

	families, err := registry.Gather()
	require.NoError(t, err)
	submissionCounts := map[string]float64{}
	var durationCount uint64
	for _, family := range families {
		switch family.GetName() {
		case "trakrf_twilio_submissions_total":
			for _, metric := range family.GetMetric() {
				require.Len(t, metric.GetLabel(), 1)
				label := metric.GetLabel()[0]
				require.Equal(t, "result", label.GetName())
				submissionCounts[label.GetValue()] = metric.GetCounter().GetValue()
			}
		case "trakrf_twilio_request_duration_seconds":
			require.Len(t, family.GetMetric(), 1)
			durationCount = family.GetMetric()[0].GetHistogram().GetSampleCount()
		case "trakrf_twilio_callbacks_total":
			t.Fatal("sender integration emitted callback metrics")
		default:
			t.Fatalf("unexpected metric family %q", family.GetName())
		}
	}
	require.Equal(t, map[string]float64{"accepted": concurrentSends + 1, "permanent": 1}, submissionCounts)
	require.Equal(t, uint64(concurrentSends+2), durationCount)
	for _, sensitive := range []string{privateDestination, privateBody, privateCredential, "SM-integration-submission"} {
		require.NotContains(t, fmt.Sprint(submissionCounts), sensitive)
	}
}
