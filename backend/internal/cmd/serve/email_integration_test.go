//go:build integration

package serve

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// Exercise the production runtime and SDK using a fake HTTP transport, signed
// callbacks and real PostgreSQL. No outgoing records or real sends are required.
func TestEmailIntegration_SubmissionAndDurableCallbacks(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	registry := prometheus.NewRegistry()
	var submissions int
	transport := emailTestTransport(func(r *http.Request) (*http.Response, error) {
		submissions++
		require.Equal(t, "https://api.resend.com/emails", r.URL.String())
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "Bearer re_fixture", r.Header.Get("Authorization"))
		require.Equal(t, fmt.Sprintf("notification-email/v1/%x", sha256.Sum256([]byte("delivery-test"))), r.Header.Get("Idempotency-Key"))
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Equal(t, map[string]any{"from": "notify@example.com", "to": []any{"private@example.com"}, "subject": "test subject", "text": "test body", "html": "<p>test body</p>"}, payload)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"email-test"}`)), Request: r}, nil
	})
	// Snapshot the fake transport during construction, restoring it before any
	// requests. Like the SMS integration test, this test must not run in parallel.
	original := http.DefaultTransport
	http.DefaultTransport = transport
	runtime, err := notification.NewEmailRuntime(serverEmailConfig(), db.Store.EmailCallbackConsumer(), registry)
	http.DefaultTransport = original
	require.NoError(t, err)
	require.Zero(t, submissions, "construction must not send")
	result, err := runtime.Sender.SendEmail(ctx, email.Command{DeliveryID: "delivery-test", To: "private@example.com", Subject: "test subject", Text: "test body", HTML: "<p>test body</p>"})
	require.NoError(t, err)
	require.Equal(t, email.Submission{ProviderMessageID: "email-test"}, result)
	require.Equal(t, 1, submissions)
	var acceptedCount int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.email_callback_events`).Scan(&acceptedCount))
	require.Zero(t, acceptedCount, "submission acceptance must not manufacture delivery")
	router := setupTestRouter(t, runtime)
	body := serverEmailBody("email.delivered", result.ProviderMessageID)
	var count int
	_, err = db.AdminPool.Exec(ctx, `REVOKE INSERT ON trakrf.email_callback_events FROM trakrf_test_app`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.AdminPool.Exec(context.Background(), `GRANT INSERT ON trakrf.email_callback_events TO trakrf_test_app`)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, serverEmailRequest(body))
	require.Equal(t, 503, rec.Code)
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.email_callback_events`).Scan(&count))
	require.Zero(t, count)
	_, err = db.AdminPool.Exec(ctx, `GRANT INSERT ON trakrf.email_callback_events TO trakrf_test_app`)
	require.NoError(t, err)
	var firstReceipt time.Time
	for attempt := range 2 {
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, serverEmailRequest(body))
		require.Equal(t, 204, rec.Code)
		var eventID, messageID, kind string
		var occurred, received time.Time
		require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT provider_event_id, provider_message_id, event_type, occurred_at, received_at FROM trakrf.email_callback_events WHERE provider='resend'`).Scan(&eventID, &messageID, &kind, &occurred, &received))
		require.Equal(t, "evt-private", eventID)
		require.Equal(t, "email-test", messageID)
		require.Equal(t, "delivered", kind)
		require.Equal(t, "2026-01-02T03:04:05.123456Z", occurred.UTC().Format(time.RFC3339Nano))
		if attempt == 0 {
			firstReceipt = received
		} else {
			require.Equal(t, firstReceipt, received)
		}
		// Reconstruct both runtime and router; replay is durable across instances.
		runtime, err = notification.NewEmailRuntime(serverEmailConfig(), db.Store.EmailCallbackConsumer(), prometheus.NewRegistry())
		require.NoError(t, err)
		router = setupTestRouter(t, runtime)
	}
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.email_callback_events`).Scan(&count))
	require.Equal(t, 1, count)
	families, err := registry.Gather()
	require.NoError(t, err)
	outcomes := map[string]float64{}
	for _, family := range families {
		if family.GetName() == "trakrf_resend_request_duration_seconds" {
			require.Equal(t, uint64(1), family.Metric[0].Histogram.GetSampleCount())
			continue
		}
		for _, metric := range family.Metric {
			labels := map[string]string{}
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			outcomes[family.GetName()+"/"+labels["result"]] = metric.Counter.GetValue()
			if family.GetName() == "trakrf_resend_submissions_total" {
				require.Equal(t, "false", labels["outcome_unknown"])
			}
		}
	}
	require.Equal(t, map[string]float64{"trakrf_resend_submissions_total/accepted": 1, "trakrf_resend_callbacks_total/consumer_failure": 1, "trakrf_resend_callbacks_total/persisted": 1}, outcomes)
}

type emailTestTransport func(*http.Request) (*http.Response, error)

func (f emailTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
