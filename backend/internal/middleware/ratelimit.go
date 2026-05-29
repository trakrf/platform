package middleware

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/models/apikey"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/ratelimit"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// apiV1Prefix gates the path-scoped rate-limit-header middleware so
// /metrics, /swagger, /openapi.* aliases, and the SPA asset routes stay
// header-free. The /api/v1 (no trailing slash) variant is included so a
// probe for the bare prefix still gets X-RateLimit-* on its 404.
const apiV1Prefix = "/api/v1"

// writeRateLimitHeaders emits the rate-limit headers from a Decision. Used by
// both RateLimit (per-key, post-auth) and DefaultRateLimitHeaders (anonymous,
// pre-auth) so header semantics live in one place.
//
// X-RateLimit-Limit is the burst ceiling and X-RateLimit-Remaining counts down
// 1:1 to it (TRA-878 Option A). The lower sustained rate is advertised in the
// separate RateLimit-Policy header as `<quota>;w=<window-seconds>`, so a client
// can distinguish "how hard can I burst" (Limit/Remaining) from "what can I
// sustain" (Policy).
func writeRateLimitHeaders(w http.ResponseWriter, d ratelimit.Decision) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(d.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(d.ResetAt.Unix(), 10))
	w.Header().Set("RateLimit-Policy", fmt.Sprintf("%d;w=%d", d.PolicyQuota, d.PolicyWindowSec))
}

// DefaultRateLimitHeaders returns a middleware that pre-emits steady-state
// rate-limit headers using the limiter's configured limit. It runs before
// authentication so every /api/v1/* response carries rate-limit headers — even
// auth-failure 401s and unknown-route 404s where the principal is unknown and
// the per-key bucket can't be consulted.
//
// When a request is later API-key-authenticated, the RateLimit middleware
// overwrites these defaults with real bucket values. When auth fails or the
// route 404s, the defaults remain — giving integration partners parseable
// X-RateLimit-* on every response. (TRA-518)
func DefaultRateLimitHeaders(lim *ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeRateLimitHeaders(w, lim.AnonDecision())
			next.ServeHTTP(w, r)
		})
	}
}

// APIv1DefaultRateLimitHeaders is DefaultRateLimitHeaders scoped to the
// public /api/v1/* path prefix, intended for use as a global middleware
// that runs before ContentType. It guarantees the three X-RateLimit-*
// headers appear on every public-API response — including 415 rejections
// from ContentType, which fire before routing and therefore before any
// per-group DefaultRateLimitHeaders the request would otherwise reach
// (TRA-703 / BB32 C1). For non-/api/v1 paths (SPA assets, /metrics,
// /swagger) the middleware is a no-op so those responses stay header-free.
//
// Per-group DefaultRateLimitHeaders is still wired on each /api/v1/* group
// because RateLimit overwrites the same headers with real per-key bucket
// values after API-key auth; the per-group call leaves a clean reset point
// after this global default in chains where authentication may not run.
func APIv1DefaultRateLimitHeaders(lim *ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == apiV1Prefix || strings.HasPrefix(r.URL.Path, apiV1Prefix+"/") {
				writeRateLimitHeaders(w, lim.AnonDecision())
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit returns a middleware that enforces per-key rate limits on
// API-key-authenticated requests. Session-authenticated requests (identified
// by the absence of an APIKeyPrincipal on the context) pass through untouched.
//
// Emits X-RateLimit-Limit, X-RateLimit-Remaining, and X-RateLimit-Reset on
// every rate-limited response. On denial, emits 429 with Retry-After and a
// standard error envelope (type=rate_limited).
//
// allowTestBypass controls whether the test-handler-minted Schemathesis key
// (apikey.SchemathesisMintKeyName) is exempt from rate limiting. The router
// passes true only when APP_ENV != "production"; that env gate is exactly the
// same guard used for mounting the test handler itself, so the bypass cannot
// activate in production even if a key with the magic name leaked into the
// prod database. TRA-677 / Schemathesis Class F.
func RateLimit(lim *ratelimit.Limiter, allowTestBypass bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := GetAPIKeyPrincipal(r)
			if p == nil {
				next.ServeHTTP(w, r)
				return
			}

			if allowTestBypass && p.Name == apikey.SchemathesisMintKeyName {
				// Headers from DefaultRateLimitHeaders (full-quota anonymous
				// defaults) remain on the response; we don't overwrite them
				// with real bucket values because this principal isn't being
				// metered. Schemathesis only branches on status, not header
				// values, so leaving the defaults is the simplest answer.
				next.ServeHTTP(w, r)
				return
			}

			d := lim.Allow(p.JTI)
			writeRateLimitHeaders(w, d)

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

					fmt.Sprintf("Retry after %d seconds", retrySec),
					reqID)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
