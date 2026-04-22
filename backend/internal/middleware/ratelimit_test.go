package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/errors"
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

func requestWithAPIKey(jti string, orgID int) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	p := &APIKeyPrincipal{OrgID: orgID, JTI: jti, Scopes: []string{"assets:read"}}
	ctx := context.WithValue(req.Context(), APIKeyPrincipalKey, p)
	return req.WithContext(ctx)
}

func TestRateLimit_AllowedRequestSetsHeaders(t *testing.T) {
	lim, _ := newTestRateLimiter(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	RateLimit(lim)(next).ServeHTTP(rec, requestWithAPIKey("jti-alpha", 42))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "60", rec.Header().Get("X-RateLimit-Limit"))
	require.Equal(t, "60", rec.Header().Get("X-RateLimit-Remaining"),
		"tokens above Limit: Remaining caps at Limit")
	require.NotEmpty(t, rec.Header().Get("X-RateLimit-Reset"))
	require.Empty(t, rec.Header().Get("Retry-After"), "allowed responses have no Retry-After")
}

func TestRateLimit_DeniedRequestReturns429WithEnvelope(t *testing.T) {
	lim, _ := newTestRateLimiter(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run when rate limit is exceeded")
	})

	// Drain the burst for this principal.
	drain := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	for i := 0; i < 120; i++ {
		rec := httptest.NewRecorder()
		RateLimit(lim)(drain).ServeHTTP(rec, requestWithAPIKey("jti-alpha", 42))
		require.Equal(t, http.StatusOK, rec.Code, "request %d should succeed", i+1)
	}

	// 121st request — denied.
	rec := httptest.NewRecorder()
	RateLimit(lim)(next).ServeHTTP(rec, requestWithAPIKey("jti-alpha", 42))

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "1", rec.Header().Get("Retry-After"))
	require.Equal(t, "60", rec.Header().Get("X-RateLimit-Limit"))
	require.Equal(t, "0", rec.Header().Get("X-RateLimit-Remaining"))

	var body struct {
		Error struct {
			Type     string `json:"type"`
			Title    string `json:"title"`
			Status   int    `json:"status"`
			Detail   string `json:"detail"`
			Instance string `json:"instance"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, string(errors.ErrRateLimited), body.Error.Type)
	require.Equal(t, "Rate limit exceeded", body.Error.Title)
	require.Equal(t, 429, body.Error.Status)
	require.Equal(t, "Retry after 1 seconds", body.Error.Detail)
	require.Equal(t, "/api/v1/assets", body.Error.Instance)
}

func TestRateLimit_HeaderInvariantsAcrossManyRequests(t *testing.T) {
	lim, _ := newTestRateLimiter(t)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Drive enough requests to move through the full range of bucket states:
	// fresh (tokens above limit), at-limit, below-limit, drained. IETF
	// RateLimit header contract requires remaining ≤ limit on every response.
	for i := 0; i < 130; i++ {
		rec := httptest.NewRecorder()
		RateLimit(lim)(next).ServeHTTP(rec, requestWithAPIKey("invariant-key", 1))

		limit, err := strconv.Atoi(rec.Header().Get("X-RateLimit-Limit"))
		require.NoErrorf(t, err, "request %d: X-RateLimit-Limit must be integer", i+1)
		remaining, err := strconv.Atoi(rec.Header().Get("X-RateLimit-Remaining"))
		require.NoErrorf(t, err, "request %d: X-RateLimit-Remaining must be integer", i+1)
		require.LessOrEqualf(t, remaining, limit,
			"request %d: X-RateLimit-Remaining=%d must be ≤ X-RateLimit-Limit=%d",
			i+1, remaining, limit)
	}
}

func TestRateLimit_TwoPrincipalsIndependent(t *testing.T) {
	lim, _ := newTestRateLimiter(t)

	drain := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Drain key-a.
	for i := 0; i < 120; i++ {
		rec := httptest.NewRecorder()
		RateLimit(lim)(drain).ServeHTTP(rec, requestWithAPIKey("key-a", 1))
	}

	// key-a denied.
	recA := httptest.NewRecorder()
	RateLimit(lim)(drain).ServeHTTP(recA, requestWithAPIKey("key-a", 1))
	require.Equal(t, http.StatusTooManyRequests, recA.Code)

	// key-b still healthy.
	recB := httptest.NewRecorder()
	RateLimit(lim)(drain).ServeHTTP(recB, requestWithAPIKey("key-b", 2))
	require.Equal(t, http.StatusOK, recB.Code)
	require.Equal(t, "60", recB.Header().Get("X-RateLimit-Remaining"))
}
