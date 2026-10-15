package serve

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/buildinfo"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/notification"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/resend"
)

var emailEnvironmentKeys = []string{"NOTIFICATION_EMAIL_ENABLED", "NOTIFICATION_EMAIL_FROM", "NOTIFICATION_EMAIL_TIMEOUT", "RESEND_API_KEY", "RESEND_WEBHOOK_SECRET"}

func TestRun_EmailConfigurationBeforeStorage(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("JWT_SECRET", strings.Repeat("test-secret-", 6))
	t.Setenv("PG_URL", "")
	for _, key := range smsEnvironmentKeys {
		t.Setenv(key, "")
	}
	for _, key := range emailEnvironmentKeys {
		t.Setenv(key, "")
	}
	for _, tc := range []struct{ name, key, value, want string }{
		{"key alone disabled", "RESEND_API_KEY", "private-key", "PG_URL environment variable not set"},
		{"invalid enable", "NOTIFICATION_EMAIL_ENABLED", "bad", "must be a boolean"},
		{"missing credentials", "NOTIFICATION_EMAIL_ENABLED", "true", "notification email API key"},
		{"invalid sender", "NOTIFICATION_EMAIL_FROM", "private-address", "sender must be"},
		{"invalid secret", "RESEND_WEBHOOK_SECRET", "private-secret", "webhook signing key"},
		{"invalid timeout", "NOTIFICATION_EMAIL_TIMEOUT", "-1s", "timeout must be positive"},
		{"valid configuration", "NOTIFICATION_EMAIL_TIMEOUT", "1s", "PG_URL environment variable not set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range emailEnvironmentKeys {
				t.Setenv(key, "")
			}
			if tc.name != "key alone disabled" && tc.name != "missing credentials" {
				t.Setenv("NOTIFICATION_EMAIL_ENABLED", "true")
				t.Setenv("RESEND_API_KEY", "private-key")
				t.Setenv("NOTIFICATION_EMAIL_FROM", "notify@example.com")
				t.Setenv("RESEND_WEBHOOK_SECRET", "whsec_c2VjcmV0")
			}
			t.Setenv(tc.key, tc.value)
			err := Run(context.Background(), buildinfo.Info{}, fstest.MapFS{})
			require.ErrorContains(t, err, tc.want)
			require.NotContains(t, err.Error(), "private-")
		})
	}
}

func TestRouter_EmailCallbacks(t *testing.T) {
	var logs bytes.Buffer
	original := *logger.Get()
	logger.SetForTest(zerolog.New(&logs).Level(zerolog.DebugLevel))
	t.Cleanup(func() { logger.SetForTest(original) })
	consumer := &serverEmailConsumer{}
	runtime, err := notification.NewEmailRuntime(serverEmailConfig(), consumer, prometheus.NewRegistry())
	require.NoError(t, err)
	router := setupTestRouter(t, runtime)
	body := serverEmailBody("email.delivered", "email-test")
	req := serverEmailRequest(body)
	signature := req.Header.Get("svix-signature")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, 204, rec.Code)
	require.Len(t, consumer.events, 1)
	require.Equal(t, "evt-private", consumer.events[0].ProviderEventID)
	require.Equal(t, "email-test", consumer.events[0].ProviderMessageID)
	require.NotContains(t, logs.String(), signature)
	require.NotContains(t, logs.String(), "evt-private")
	require.NotContains(t, logs.String(), "private@example.com")
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPut} {
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, notification.EmailCallbackPath, nil))
		require.Equal(t, 405, rec.Code)
		require.Equal(t, "POST", rec.Header().Get("Allow"))
	}
	req = serverEmailRequest(body)
	req.Header.Set("svix-signature", "forged")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, 403, rec.Code)
	require.Len(t, consumer.events, 1)
	consumer.err = errors.New("private storage detail")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, serverEmailRequest(body))
	require.Equal(t, 503, rec.Code)
	require.NotContains(t, rec.Body.String(), "private")
	disabled, err := notification.NewEmailRuntime(resend.Config{}, nil, nil)
	require.NoError(t, err)
	rec = httptest.NewRecorder()
	setupTestRouter(t, disabled).ServeHTTP(rec, serverEmailRequest(body))
	require.Equal(t, 404, rec.Code)
}

func serverEmailConfig() resend.Config {
	return resend.Config{Enabled: true, APIKey: "re_fixture", From: "notify@example.com", WebhookSecret: "whsec_c2VjcmV0", Timeout: time.Second}
}
func serverEmailBody(kind, messageID string) string {
	return fmt.Sprintf(`{"type":%q,"created_at":"2026-01-02T03:04:05.123456Z","data":{"email_id":%q,"to":["private@example.com"]}}`, kind, messageID)
}
func serverEmailRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, notification.EmailCallbackPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("svix-id", "evt-private")
	req.Header.Set("svix-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	mac := hmac.New(sha256.New, []byte("secret"))
	fmt.Fprintf(mac, "%s.%s.%s", req.Header.Get("svix-id"), req.Header.Get("svix-timestamp"), body)
	req.Header.Set("svix-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return req
}

type serverEmailConsumer struct {
	events []email.CallbackEvent
	err    error
}

func (c *serverEmailConsumer) HandleEvent(_ context.Context, e email.CallbackEvent) error {
	c.events = append(c.events, e)
	return c.err
}
