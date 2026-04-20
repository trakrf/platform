package middleware

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/ratelimit"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// RateLimit returns a middleware that enforces per-key rate limits on
// API-key-authenticated requests. Session-authenticated requests (identified
// by the absence of an APIKeyPrincipal on the context) pass through untouched.
//
// Emits X-RateLimit-Limit, X-RateLimit-Remaining, and X-RateLimit-Reset on
// every rate-limited response. On denial, emits 429 with Retry-After and a
// standard error envelope (type=rate_limited).
func RateLimit(lim *ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := GetAPIKeyPrincipal(r)
			if p == nil {
				next.ServeHTTP(w, r)
				return
			}

			d := lim.Allow(p.JTI)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(d.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(d.ResetAt.Unix(), 10))

			if !d.Allowed {
				retrySec := int(math.Ceil(d.RetryAfter.Seconds()))
				if retrySec < 1 {
					retrySec = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retrySec))

				reqID := GetRequestID(r.Context())
				logger.Get().Warn().
					Str("request_id", reqID).
					Str("jti", p.JTI).
					Int("org_id", p.OrgID).
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Msg("rate limit exceeded")

				httputil.WriteJSONError(w, r, http.StatusTooManyRequests,
					errors.ErrRateLimited,
					"Rate limit exceeded",
					fmt.Sprintf("Retry after %d seconds", retrySec),
					reqID)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
