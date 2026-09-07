package twilio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
)

// This fails if public sender construction loses its optional-recorder behavior
// at the observable pre-submit cancellation boundary.
func TestNewSender_MetricsConstruction(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := NewMetrics(registry)
	require.NoError(t, err)

	tests := []struct {
		name               string
		build              func() (*Sender, error)
		wantUnknownMetrics bool
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
			wantUnknownMetrics: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sender, err := test.build()

			require.NoError(t, err)
			require.NotNil(t, sender)
			cause := errors.New("caller cancelled before submission")
			ctx, cancel := context.WithCancelCause(context.Background())
			cancel(cause)

			submission, err := sender.SendSMS(ctx, sms.Command{ToE164: sensitiveDestination, Body: sensitiveBody})

			require.Equal(t, sms.Submission{}, submission)
			require.ErrorIs(t, err, cause)

			if !test.wantUnknownMetrics {
				return
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
					require.Equal(t, "result", metric.GetLabel()[0].GetName())
					require.Equal(t, metricSubmissionResultUnknown, metric.GetLabel()[0].GetValue())
				case "trakrf_twilio_request_duration_seconds":
					sawDuration = true
					require.Len(t, family.GetMetric(), 1)
					require.Equal(t, uint64(0), family.GetMetric()[0].GetHistogram().GetSampleCount())
				default:
					t.Fatalf("unexpected metric family %q", family.GetName())
				}
			}
			require.True(t, sawSubmission)
			require.True(t, sawDuration)
		})
	}
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
			var providerErr *sms.ProviderError
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
			var providerErr *sms.ProviderError
			require.ErrorAs(t, err, &providerErr)
			require.Equal(t, sms.ProviderError{Kind: sms.ErrorPermanent, Code: unknownCode}, *providerErr)

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
