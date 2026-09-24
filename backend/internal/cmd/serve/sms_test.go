package serve

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/buildinfo"
	"github.com/trakrf/platform/backend/internal/notification"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/trakrf/platform/backend/internal/notification/twilio"
)

func TestRun_RejectsPartialSMSConfigurationBeforeStorage(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("JWT_SECRET", strings.Repeat("test-secret-", 6))
	t.Setenv("PG_URL", "")
	for _, key := range smsEnvironmentKeys {
		t.Setenv(key, "")
	}
	t.Setenv("TWILIO_API_KEY_SID", "SKpartial")
	err := Run(context.Background(), buildinfo.Info{}, fstest.MapFS{})
	require.ErrorContains(t, err, "Twilio configuration is incomplete")
	t.Setenv("TWILIO_API_KEY_SID", "")
	err = Run(context.Background(), buildinfo.Info{}, fstest.MapFS{})
	require.ErrorContains(t, err, "PG_URL environment variable not set")
}

// Mounting must preserve form support and signature authentication in the
// actual root router, bypassing session auth and JSON-only middleware.
func TestRouter_SMSCallbacks(t *testing.T) {
	consumer := &serverSMSConsumer{}
	runtime, err := notification.NewRuntime(serverSMSConfig(), consumer, prometheus.NewRegistry())
	require.NoError(t, err)
	router := setupTestRouter(t, runtime)
	for _, tc := range []struct {
		path string
		form url.Values
	}{
		{"/api/v1/notifications/twilio/status", url.Values{"MessageSid": {"SMsent"}, "MessageStatus": {"delivered"}}},
		{"/api/v1/notifications/twilio/inbound", url.Values{"MessageSid": {"SMreceived"}, "From": {"+15555550123"}, "To": {"+15555550456"}, "Body": {"ARRET"}, "OptOutType": {"STOP"}}},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, serverSMSRequest(tc.path, tc.form))
		require.Equal(t, 204, rec.Code)
		forged := serverSMSRequest(tc.path, tc.form)
		forged.Header.Set("X-Twilio-Signature", "forged")
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, forged)
		require.Equal(t, 403, rec.Code)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		require.Equal(t, 405, rec.Code)
		require.Equal(t, "POST", rec.Header().Get("Allow"))
	}
	require.Equal(t, 1, consumer.statuses)
	require.Equal(t, 1, consumer.keywords)
	consumer.err = errors.New("storage unavailable")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, serverSMSRequest("/api/v1/notifications/twilio/status", url.Values{"MessageSid": {"SMsent"}, "MessageStatus": {"sent"}}))
	require.Equal(t, 500, rec.Code)
	// Disabled production composition has no active callbacks.
	rec = httptest.NewRecorder()
	setupTestRouter(t).ServeHTTP(rec, serverSMSRequest("/api/v1/notifications/twilio/status", url.Values{}))
	require.Equal(t, 404, rec.Code)
}

var smsEnvironmentKeys = []string{"TWILIO_ACCOUNT_SID", "TWILIO_API_KEY_SID", "TWILIO_API_KEY_SECRET", "TWILIO_AUTH_TOKEN", "TWILIO_MESSAGING_SERVICE_SID", "TWILIO_PUBLIC_BASE_URL"}

func serverSMSConfig() twilio.Config {
	return twilio.Config{AccountSID: "ACtest", APIKeySID: "SKtest", APIKeySecret: "secret", AuthToken: "token", MessagingServiceSID: "MGtest", PublicBaseURL: "https://callbacks.example.com"}
}

func serverSMSRequest(path string, form url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mac := hmac.New(sha1.New, []byte("token"))
	mac.Write([]byte("https://callbacks.example.com" + path))
	keys := make([]string, 0, len(form))
	for key := range form {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		mac.Write([]byte(key + form.Get(key)))
	}
	req.Header.Set("X-Twilio-Signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return req
}

type serverSMSConsumer struct {
	statuses, keywords int
	err                error
}

func (c *serverSMSConsumer) HandleStatus(context.Context, sms.ProviderStatus) error {
	c.statuses++
	return c.err
}
func (c *serverSMSConsumer) HandleKeyword(context.Context, sms.InboundKeyword) error {
	c.keywords++
	return c.err
}
