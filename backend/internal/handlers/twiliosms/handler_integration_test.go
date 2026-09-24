package twiliosms

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/trakrf/platform/backend/internal/notification/twilio"
)

type integrationCallbackConsumer struct {
	mu       sync.Mutex
	statuses []sms.ProviderStatus
	keywords []sms.InboundKeyword
}

func (c *integrationCallbackConsumer) HandleStatus(_ context.Context, status sms.ProviderStatus) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statuses = append(c.statuses, status)
	return nil
}

func (c *integrationCallbackConsumer) HandleKeyword(_ context.Context, keyword sms.InboundKeyword) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keywords = append(c.keywords, keyword)
	return nil
}

// This integration check joins configured construction, public Chi routes,
// actual signature validation, downstream handoff, and bounded metrics. The
// callback boundary does not emit logs; privacy is therefore demonstrated by
// captured domain events, fixed HTTP bodies, and gathered metric labels.
func TestHandlerIntegration_RegistersSignedCallbacksAndPreservesPrivacy(t *testing.T) {
	const (
		privateStatusID = "SM-private-delivery-id"
		privateFrom     = "+15555550991"
		privateTo       = "+15555550992"
		privateText     = "a private unrelated inbound message"
	)

	registry := prometheus.NewRegistry()
	metrics, err := twilio.NewMetrics(registry)
	require.NoError(t, err)

	consumer := &integrationCallbackConsumer{}
	handler, err := NewHandlerWithMetrics(twilio.Config{
		AccountSID:          "AC-callback-integration",
		APIKeySID:           "SK-callback-integration",
		APIKeySecret:        "callback-integration-secret",
		AuthToken:           testAuthToken,
		MessagingServiceSID: "MG-callback-integration",
		PublicBaseURL:       testPublicBaseURL,
	}, consumer, metrics)
	require.NoError(t, err)
	callbackTime := time.Date(2026, time.August, 31, 12, 30, 0, 0, time.FixedZone("callback", -4*60*60))
	handler.now = func() time.Time { return callbackTime }

	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	serve := func(request *http.Request) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	statusForm := url.Values{"MessageSid": {privateStatusID}, "MessageStatus": {"delivered"}}
	for range 2 {
		require.Equal(t, http.StatusNoContent, serve(signedStatusRequest(t, statusForm)).Code)
	}
	require.Equal(t, http.StatusNoContent, serve(signedInboundRequest(t, url.Values{
		"MessageSid": {"SM-private-stop-id"}, "From": {privateFrom}, "To": {privateTo}, "Body": {" STOP "},
	})).Code)
	require.Equal(t, http.StatusNoContent, serve(signedInboundRequest(t, url.Values{
		"MessageSid": {"SM-private-start-id"}, "From": {privateFrom}, "To": {privateTo}, "Body": {"start"},
	})).Code)
	unrelatedResponse := serve(signedInboundRequest(t, url.Values{"Body": {privateText}}))
	require.Equal(t, http.StatusNoContent, unrelatedResponse.Code)
	require.NotContains(t, unrelatedResponse.Body.String(), privateText)

	forged := httptest.NewRequest(http.MethodPost, statusCallbackPath, strings.NewReader("MessageSid=SM-forged&MessageStatus=delivered"))
	forged.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	forged.Header.Set("X-Twilio-Signature", "forged-signature")
	forgedResponse := serve(forged)
	require.Equal(t, http.StatusForbidden, forgedResponse.Code)
	require.NotContains(t, forgedResponse.Body.String(), "SM-forged")

	consumer.mu.Lock()
	require.Equal(t, []sms.ProviderStatus{
		{ProviderMessageID: privateStatusID, Status: "delivered", OccurredAt: callbackTime.UTC()},
		{ProviderMessageID: privateStatusID, Status: "delivered", OccurredAt: callbackTime.UTC()},
	}, consumer.statuses)
	require.Equal(t, []sms.InboundKeyword{
		{ProviderMessageID: "SM-private-stop-id", FromE164: privateFrom, ToE164: privateTo, Keyword: "STOP", ReceivedAt: callbackTime.UTC()},
		{ProviderMessageID: "SM-private-start-id", FromE164: privateFrom, ToE164: privateTo, Keyword: "START", ReceivedAt: callbackTime.UTC()},
	}, consumer.keywords)
	consumer.mu.Unlock()

	families, err := registry.Gather()
	require.NoError(t, err)
	callbackCounts := map[string]float64{}
	var durationCount uint64
	for _, family := range families {
		switch family.GetName() {
		case "trakrf_twilio_callbacks_total":
			for _, metric := range family.GetMetric() {
				labels := map[string]string{}
				for _, label := range metric.GetLabel() {
					labels[label.GetName()] = label.GetValue()
				}
				require.Len(t, labels, 2)
				callbackCounts[labels["type"]+"/"+labels["result"]] = metric.GetCounter().GetValue()
			}
		case "trakrf_twilio_request_duration_seconds":
			require.Len(t, family.GetMetric(), 1)
			durationCount = family.GetMetric()[0].GetHistogram().GetSampleCount()
		case "trakrf_twilio_submissions_total":
			t.Fatal("callback integration emitted submission metrics")
		default:
			t.Fatalf("unexpected metric family %q", family.GetName())
		}
	}
	require.Equal(t, map[string]float64{
		"status/accepted":          2,
		"status/invalid_signature": 1,
		"inbound/accepted":         3,
	}, callbackCounts)
	require.Equal(t, uint64(6), durationCount)
	for _, sensitive := range []string{privateStatusID, privateFrom, privateTo, privateText, "SM-private-stop-id", "SM-private-start-id", testAuthToken} {
		require.NotContains(t, fmt.Sprint(callbackCounts), sensitive)
	}
}
