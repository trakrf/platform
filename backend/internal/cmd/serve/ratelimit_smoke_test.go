package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	"github.com/trakrf/platform/backend/internal/ratelimit"
)

// TestRateLimit_MountedOnEitherAuthGroup verifies that a handler mounted under
// the same group shape as the public API read endpoints receives rate-limit
// headers when invoked with an APIKeyPrincipal on the context.
//
// This is an isolated smoke test — it does not boot the full router (which
// requires DB + storage) but exercises the same middleware chain shape.
func TestRateLimit_MountedOnEitherAuthGroup(t *testing.T) {
	clock := ratelimit.NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
	lim := ratelimit.NewLimiter(ratelimit.Config{
		RatePerMinute: 60,
		Burst:         120,
		IdleTTL:       time.Hour,
		SweepInterval: 24 * time.Hour,
		Clock:         clock,
	})
	defer lim.Close()

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(middleware.RateLimit(lim))
		r.Get("/ping", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	p := &middleware.APIKeyPrincipal{OrgID: 7, JTI: "smoke-jti", Scopes: []string{"assets:read"}}
	req = req.WithContext(contextWithPrincipal(req.Context(), p))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "60", rec.Header().Get("X-RateLimit-Limit"))
	require.Equal(t, "119", rec.Header().Get("X-RateLimit-Remaining"))
}

// contextWithPrincipal mirrors what APIKeyAuth does internally. Kept inline
// rather than exported from middleware to avoid widening the package surface.
func contextWithPrincipal(ctx context.Context, p *middleware.APIKeyPrincipal) context.Context {
	return context.WithValue(ctx, middleware.APIKeyPrincipalKey, p)
}
