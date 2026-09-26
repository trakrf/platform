//go:build integration

package resendemail

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestWebhookDurableAcknowledgment(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	h, err := NewHandler(config(), db.Store.EmailCallbackConsumer())
	require.NoError(t, err)
	body := fmt.Sprintf(fixture, "email.delivered")
	// A failed commit must be retryable; replay with the same authenticated ID
	// succeeds once storage is available, then remains successful as a duplicate.
	_, err = db.AdminPool.Exec(ctx, `REVOKE INSERT ON trakrf.email_callback_events FROM trakrf_test_app`)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signed(body, time.Now()))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var count int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.email_callback_events`).Scan(&count))
	require.Zero(t, count)
	_, err = db.AdminPool.Exec(ctx, `GRANT INSERT ON trakrf.email_callback_events TO trakrf_test_app`)
	require.NoError(t, err)
	for range 2 {
		// Recreate handler and consumer to establish persistence beyond one instance.
		h, err = NewHandler(config(), db.Store.EmailCallbackConsumer())
		require.NoError(t, err)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, signed(body, time.Now()))
		require.Equal(t, http.StatusNoContent, w.Code)
		require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.email_callback_events WHERE provider='resend' AND provider_event_id='evt-authenticated' AND provider_message_id='email-123' AND event_type='delivered'`).Scan(&count))
		require.Equal(t, 1, count)
	}
}
