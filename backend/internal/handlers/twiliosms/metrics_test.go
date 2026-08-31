package twiliosms

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/trakrf/platform/backend/internal/notification/twilio"
)

// This fails if a callback outcome is not recorded exactly once with its
// bounded endpoint/result labels and one callback-boundary duration.
func TestCallbacks_RecordBoundedMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	metrics, err := twilio.NewMetrics(registry)
	require.NoError(t, err)

	var typedNilInboundConsumer *inboundConsumer
	for _, test := range []struct {
		name     string
		consumer sms.CallbackConsumer
		handle   func(*Handler)
	}{
		{
			name:     "accepted status",
			consumer: &statusConsumer{},
			handle: func(handler *Handler) {
				handler.Status(httptest.NewRecorder(), signedStatusRequest(t, url.Values{
					"MessageSid":    {"SM-sensitive-id"},
					"MessageStatus": {"delivered"},
					"ErrorCode":     {"super-private-error-code"},
				}))
			},
		},
		{
			name:     "invalid status signature",
			consumer: &statusConsumer{},
			handle: func(handler *Handler) {
				req := httptest.NewRequest(http.MethodPost, statusCallbackPath, strings.NewReader("MessageSid=SM-sensitive-id&MessageStatus=delivered"))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("X-Twilio-Signature", "forged-credentials-secret")
				handler.Status(httptest.NewRecorder(), req)
			},
		},
		{
			name:     "malformed signed status",
			consumer: &statusConsumer{},
			handle: func(handler *Handler) {
				handler.Status(httptest.NewRecorder(), signedStatusRequest(t, url.Values{
					"MessageStatus": {"delivered"},
				}))
			},
		},
		{
			name:     "nil status consumer",
			consumer: nil,
			handle: func(handler *Handler) {
				handler.Status(httptest.NewRecorder(), signedStatusRequest(t, url.Values{
					"MessageSid":    {"SM-sensitive-id"},
					"MessageStatus": {"delivered"},
				}))
			},
		},
		{
			name:     "failing status consumer",
			consumer: &statusConsumer{err: errors.New("private consumer error")},
			handle: func(handler *Handler) {
				handler.Status(httptest.NewRecorder(), signedStatusRequest(t, url.Values{
					"MessageSid":    {"SM-sensitive-id"},
					"MessageStatus": {"delivered"},
				}))
			},
		},
		{
			name:     "accepted unrelated inbound",
			consumer: &inboundConsumer{},
			handle: func(handler *Handler) {
				handler.Inbound(httptest.NewRecorder(), signedInboundRequest(t, url.Values{
					"Body": {"private message body"},
				}))
			},
		},
		{
			name:     "invalid inbound signature",
			consumer: &inboundConsumer{},
			handle: func(handler *Handler) {
				req := httptest.NewRequest(http.MethodPost, inboundCallbackPath, strings.NewReader("Body=private+message+body"))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("X-Twilio-Signature", "forged-credentials-secret")
				handler.Inbound(httptest.NewRecorder(), req)
			},
		},
		{
			name:     "malformed signed inbound",
			consumer: &inboundConsumer{},
			handle: func(handler *Handler) {
				handler.Inbound(httptest.NewRecorder(), signedInboundRequest(t, url.Values{
					"MessageSid": {"SM-sensitive-id"},
					"To":         {"+15550002222"},
					"Body":       {"STOP"},
				}))
			},
		},
		{
			name:     "typed nil inbound consumer",
			consumer: typedNilInboundConsumer,
			handle: func(handler *Handler) {
				handler.Inbound(httptest.NewRecorder(), signedInboundRequest(t, inboundKeywordForm("STOP")))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, err := NewHandlerWithMetrics(newSignatureTestConfig(), test.consumer, metrics)
			require.NoError(t, err)
			test.handle(handler)
		})
	}

	families, err := registry.Gather()
	require.NoError(t, err)

	callbackCounts := map[string]float64{}
	for _, family := range families {
		switch family.GetName() {
		case "trakrf_twilio_callbacks_total":
			for _, metric := range family.GetMetric() {
				labels := map[string]string{}
				for _, label := range metric.GetLabel() {
					labels[label.GetName()] = label.GetValue()
				}
				require.Len(t, labels, 2)
				require.Contains(t, []string{"status", "inbound", "unknown"}, labels["type"])
				require.Contains(t, []string{"accepted", "invalid_signature", "malformed", "consumer_failure", "unknown"}, labels["result"])
				for _, sensitive := range []string{"SM-sensitive-id", "+1555000", "private message body", "private consumer error", "super-private-error-code", "credentials-secret"} {
					require.NotContains(t, labels["type"], sensitive)
					require.NotContains(t, labels["result"], sensitive)
				}
				callbackCounts[labels["type"]+"/"+labels["result"]] = metric.GetCounter().GetValue()
			}
		case "trakrf_twilio_request_duration_seconds":
			require.Len(t, family.GetMetric(), 1)
			require.Empty(t, family.GetMetric()[0].GetLabel())
			require.Equal(t, uint64(9), family.GetMetric()[0].GetHistogram().GetSampleCount())
		case "trakrf_twilio_submissions_total":
			t.Fatal("callback handling must not record submission metrics")
		default:
			t.Fatalf("unexpected metric family %q", family.GetName())
		}
	}

	require.Equal(t, map[string]float64{
		"status/accepted":           1,
		"status/invalid_signature":  1,
		"status/malformed":          1,
		"status/consumer_failure":   2,
		"inbound/accepted":          1,
		"inbound/invalid_signature": 1,
		"inbound/malformed":         1,
		"inbound/consumer_failure":  1,
	}, callbackCounts)
}
