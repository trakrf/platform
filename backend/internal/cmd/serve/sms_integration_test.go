//go:build integration

package serve

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// Exercise the configured runtime's real SDK sender and production router
// against a local transport and a real PostgreSQL consumer. No provider access
// or real credentials are involved; callback success requires durable storage.
func TestSMSIntegration_SubmissionAndDurableCallbacks(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	config := serverSMSConfig()
	var submissions int
	transport := smsTestTransport(func(r *http.Request) (*http.Response, error) {
		submissions++
		require.Equal(t, "api.twilio.com", r.URL.Host)
		require.Equal(t, "/2010-04-01/Accounts/ACtest/Messages.json", r.URL.Path)
		user, password, ok := r.BasicAuth()
		require.True(t, ok)
		require.Equal(t, "SKtest", user)
		require.Equal(t, "secret", password)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		form, err := url.ParseQuery(string(body))
		require.NoError(t, err)
		require.Equal(t, "MGtest", form.Get("MessagingServiceSid"))
		require.Equal(t, "+15555550123", form.Get("To"))
		require.Empty(t, form.Get("From"))
		require.Equal(t, "https://callbacks.example.com/api/v1/notifications/twilio/status#rc=3&rp=ct,rt,5xx", form.Get("StatusCallback"))
		return &http.Response{StatusCode: 201, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"sid":"SMsent","status":"accepted"}`)), Request: r}, nil
	})
	// The sender snapshots the default transport at construction. Restore the
	// global immediately, before exercising the runtime or starting requests.
	original := http.DefaultTransport
	http.DefaultTransport = transport
	runtime, err := notification.NewRuntime(config, db.Store.SMSCallbackConsumer(config.AccountSID, config.MessagingServiceSID), prometheus.NewRegistry())
	http.DefaultTransport = original
	require.NoError(t, err)
	result, err := runtime.Sender.SendSMS(ctx, sms.Command{DeliveryID: "delivery-test", ToE164: "+15555550123", Body: "test message"})
	require.NoError(t, err)
	require.Equal(t, sms.Submission{ProviderMessageID: "SMsent", Status: "accepted"}, result)
	require.Equal(t, 1, submissions)
	router := setupTestRouter(t, runtime)
	for _, callback := range []struct {
		path string
		form url.Values
	}{
		{"/api/v1/notifications/twilio/status", url.Values{"MessageSid": {"SMsent"}, "MessageStatus": {"delivered"}}},
		{"/api/v1/notifications/twilio/inbound", url.Values{"MessageSid": {"SMstop"}, "From": {"+15555550123"}, "To": {"+15555550456"}, "Body": {"ARRET"}, "OptOutType": {"STOP"}}},
	} {
		for range 2 {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, serverSMSRequest(callback.path, callback.form))
			require.Equal(t, 204, rec.Code)
		}
	}
	var count int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.sms_callback_events`).Scan(&count))
	require.Equal(t, 2, count)
	// A storage outage must never be acknowledged as a successful handoff.
	_, err = db.AdminPool.Exec(ctx, `ALTER TABLE trakrf.sms_callback_events RENAME TO sms_callback_events_unavailable`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.AdminPool.Exec(context.Background(), `ALTER TABLE trakrf.sms_callback_events_unavailable RENAME TO sms_callback_events`)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, serverSMSRequest("/api/v1/notifications/twilio/status", url.Values{"MessageSid": {"SMsent"}, "MessageStatus": {"sent"}}))
	require.Equal(t, 500, rec.Code)
	require.NotContains(t, rec.Body.String(), "sms_callback_events")
}

type smsTestTransport func(*http.Request) (*http.Response, error)

func (f smsTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
