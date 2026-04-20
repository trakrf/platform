package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/ratelimit"
)

func newTestRateLimiter(t *testing.T) (*ratelimit.Limiter, *ratelimit.FakeClock) {
	t.Helper()
	clock := ratelimit.NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := ratelimit.NewLimiter(ratelimit.Config{
		RatePerMinute: 60,
		Burst:         120,
		IdleTTL:       time.Hour,
		SweepInterval: 24 * time.Hour,
		Clock:         clock,
	})
	t.Cleanup(func() { lim.Close() })
	return lim, clock
}

func TestRateLimit_SessionAuthBypassesRateLimiting(t *testing.T) {
	lim, _ := newTestRateLimiter(t)

	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// No APIKeyPrincipal on context — simulates session-authenticated request.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	rec := httptest.NewRecorder()

	RateLimit(lim)(next).ServeHTTP(rec, req)

	require.True(t, handlerCalled, "session auth request must pass through")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Get("X-RateLimit-Limit"), "no rate-limit headers for session auth")
	require.Empty(t, rec.Header().Get("X-RateLimit-Remaining"))
	require.Empty(t, rec.Header().Get("X-RateLimit-Reset"))
}
