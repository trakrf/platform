//go:build integration

package serve

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestEmailIntegration_DurableCallbacks(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	registry := prometheus.NewRegistry()
	runtime, err := notification.NewEmailRuntime(serverEmailConfig(), db.Store.EmailCallbackConsumer(), registry)
	require.NoError(t, err)
	router := setupTestRouter(t, runtime)
	body := serverEmailBody("email.delivered", "email-test")
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
}
