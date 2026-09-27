package notification

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/resend"
)

// A runtime that loses a provider error or records it as an acceptance would
// mislead both the future delivery worker and operators monitoring failures.
func TestEmailRuntime_SubmissionFailuresRemainVisible(t *testing.T) {
	for _, tc := range []struct {
		name, outcome, unknown string
		status                 int
		want                   email.ProviderError
	}{
		{"rejected", "permanent", "false", 400, email.ProviderError{Kind: email.ErrorPermanent, HTTPStatus: 400}},
		{"rate limited", "transient", "false", 429, email.ProviderError{Kind: email.ErrorTransient, HTTPStatus: 429}},
		{"ambiguous", "transient", "true", 503, email.ProviderError{Kind: email.ErrorTransient, HTTPStatus: 503, OutcomeUnknown: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Keep the runtime, adapter, SDK and metrics real; replace only HTTP.
			original := http.DefaultTransport
			t.Cleanup(func() { http.DefaultTransport = original })
			calls := 0
			http.DefaultTransport = emailRuntimeTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"message":"private provider detail"}`))}, nil
			})
			registry := prometheus.NewRegistry()
			runtime, err := NewEmailRuntime(resend.Config{Enabled: true, APIKey: "fixture", From: "notify@example.com", WebhookSecret: "whsec_c2VjcmV0", Timeout: time.Second}, &emailRuntimeConsumer{}, registry)
			require.NoError(t, err)
			http.DefaultTransport = original
			result, err := runtime.Sender.SendEmail(context.Background(), email.Command{DeliveryID: "delivery-private", To: "private@example.com", Subject: "private subject", Text: "private body"})
			require.Equal(t, email.Submission{}, result)
			require.Equal(t, &tc.want, err)
			require.Equal(t, 1, calls, "runtime must not retry submissions")
			families, err := registry.Gather()
			require.NoError(t, err)
			foundCounter, foundDuration := false, false
			for _, family := range families {
				require.NotContains(t, family.String(), "private")
				switch family.GetName() {
				case "trakrf_resend_submissions_total":
					foundCounter = true
					require.Len(t, family.Metric, 1, "a failure must not also count as accepted")
					metric := family.Metric[0]
					labels := map[string]string{}
					for _, label := range metric.Label {
						labels[label.GetName()] = label.GetValue()
					}
					require.Equal(t, map[string]string{"result": tc.outcome, "outcome_unknown": tc.unknown}, labels)
					require.Equal(t, float64(1), metric.Counter.GetValue())
				case "trakrf_resend_request_duration_seconds":
					foundDuration = true
					require.Equal(t, uint64(1), family.Metric[0].Histogram.GetSampleCount())
				}
			}
			require.True(t, foundCounter, "submission failure counter missing")
			require.True(t, foundDuration, "failed submission duration missing")
		})
	}
}

type emailRuntimeTransport func(*http.Request) (*http.Response, error)

func (f emailRuntimeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
