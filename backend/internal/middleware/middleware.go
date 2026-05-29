package middleware

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/oklog/ulid/v2"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

type contextKey string

const requestIDKey contextKey = "requestID"
const UserClaimsKey contextKey = "user_claims"

// RequestID generates or extracts a request ID and injects it into the context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Recovery catches panics and returns a 500 error response.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				requestID := GetRequestID(r.Context())

				// Use zerolog instead of slog
				logger.Get().Error().
					Interface("error", err).
					Str("request_id", requestID).
					Str("path", r.URL.Path).
					Str("method", r.Method).
					Msg("Panic recovered")

				httputil.WriteJSONError(w, r, http.StatusInternalServerError,
					errors.ErrInternal, "Internal server error", requestID)

			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORS handles Cross-Origin Resource Sharing headers.
//
// TRA-685 F10: OPTIONS short-circuit (204) is the CORS-preflight response
// and is only emitted when CORS is enabled. When BACKEND_CORS_ORIGIN is set
// to "disabled", OPTIONS is treated like any other unsupported verb — the
// request falls through to chi, which calls the root MethodNotAllowed
// handler and returns 405 with a proper `Allow` header (matching the
// existing 405 behavior on PUT/POST/etc. against read-only routes).
// Returning 204 with neither CORS headers nor `Allow` was worst-of-both.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := os.Getenv("BACKEND_CORS_ORIGIN")
		if origin == "" {
			origin = "*"
		}

		corsEnabled := origin != "disabled"
		if corsEnabled {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			// TRA-866: match the actual route table — HEAD is valid on every GET
			// route (chi auto-serves it) and no route uses PUT. The prior list
			// was a stale generic default that advertised PUT and omitted HEAD.
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "3600")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// bulkCSVUploadPath is the only route that accepts multipart/form-data;
// every other write endpoint declares application/json or
// application/merge-patch+json in the OpenAPI spec. The path is hardcoded
// here so the global ContentType middleware can grant the multipart
// exception without coupling to the assets handler. The endpoint is tagged
// internal and is not part of the public OpenAPI surface.
const bulkCSVUploadPath = "/api/v1/assets/bulk"

// oauthTokenPath additionally accepts application/x-www-form-urlencoded (on top
// of application/json) so stock OAuth2 client libraries — which default to
// form-urlencoded for the token endpoint per RFC 6749 §3.2/§4.4 — can exchange
// credentials without hand-setting a JSON content type. Every other public POST
// remains JSON-only.
const oauthTokenPath = "/api/v1/oauth/token"

// ContentType enforces declared Content-Type per method (BB32 D4 / TRA-703).
// The public docs commit to a strict per-method matrix on every write
// endpoint, and missing or otherwise-unlisted Content-Type returns 415 with
// no carve-outs:
//
//   - POST, PUT   → application/json (charset=utf-8 parameter accepted)
//   - PATCH       → application/json OR application/merge-patch+json
//   - GET, DELETE → not checked (no request body)
//
// Per-route RequireMergePatchCT further narrows declared PATCH endpoints to
// merge-patch+json only; the global middleware accepts both on PATCH so
// PATCH probes against POST-only paths still surface chi's 405 instead of
// being intercepted with 415 here.
//
// The internal bulk-CSV upload (POST /api/v1/assets/bulk) requires
// multipart/form-data and is the only path on which that media type is
// accepted. Sending multipart to any public POST endpoint returns 415,
// matching the public docs' "any other media type … returns 415 regardless
// of method" promise.
func ContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
			next.ServeHTTP(w, r)
			return
		}

		ct := r.Header.Get("Content-Type")

		if r.Method == http.MethodPost && r.URL.Path == bulkCSVUploadPath {
			if strings.HasPrefix(ct, "multipart/form-data") {
				next.ServeHTTP(w, r)
				return
			}
			httputil.Respond415(w, r, GetRequestID(r.Context()))
			return
		}

		isJSON := ct == "application/json" || ct == "application/json; charset=utf-8"
		isMergePatch := ct == "application/merge-patch+json" || ct == "application/merge-patch+json; charset=utf-8"

		// OAuth2 token endpoint accepts form-urlencoded (RFC 6749) in addition
		// to JSON; the handler branches on Content-Type to parse accordingly.
		if r.Method == http.MethodPost && r.URL.Path == oauthTokenPath {
			if isJSON || strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
				next.ServeHTTP(w, r)
				return
			}
			httputil.Respond415(w, r, GetRequestID(r.Context()))
			return
		}

		allowed := false
		switch r.Method {
		case http.MethodPost, http.MethodPut:
			allowed = isJSON
		case http.MethodPatch:
			allowed = isJSON || isMergePatch
		}

		if !allowed {
			httputil.Respond415(w, r, GetRequestID(r.Context()))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RejectQueryParams returns a per-route middleware that rejects requests
// carrying any query parameter whose key is not in `allowed`. Attach to
// endpoints that do not run through httputil.ParseListParams (single-resource
// GETs, write endpoints, subresource POST/DELETEs) so unknown query keys
// surface as 400 validation_error instead of being silently ignored —
// honoring the docs claim that "unknown query parameters are rejected with
// validation_error alongside unknown body keys" (TRA-707 / BB32 D5).
//
// Pass no arguments when the endpoint accepts no query parameters at all.
// The list endpoints already enforce this through ParseListParams and must
// NOT be wrapped, otherwise their legitimate filter/sort/limit/offset keys
// would be rejected.
func RejectQueryParams(allowed ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := httputil.RejectUnknownQueryParams(r, allowed...); err != nil {
				httputil.RespondListParamError(w, r, err, GetRequestID(r.Context()))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireMergePatchCT is a per-route middleware that enforces the
// PATCH-strict content-type. Attach to PATCH handlers so the public spec's
// declared `application/merge-patch+json` is the only accepted CT on those
// operations (RFC 7396; BB28 W2/S4). Empty Content-Type returns 415 to
// match the BB32 D4 / TRA-703 rule that every wrong Content-Type (including
// missing header) returns 415.
//
// This is per-route rather than global so PATCH probes against paths
// without a registered PATCH handler (POST-only /tags, /rename subpaths)
// produce chi's natural 405 instead of being intercepted with a 415.
func RequireMergePatchCT(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "application/merge-patch+json" && ct != "application/merge-patch+json; charset=utf-8" {
			httputil.Respond415(w, r, GetRequestID(r.Context()))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// missingAuthDetail returns the 401 detail string for a missing Authorization
// header, substituting a hint toward the correct header format when the
// request carries X-API-Key. Docs call the credential an "API key," so
// integrators often try X-API-Key first and chase a credential-rotation red
// herring on the resulting 401.
func missingAuthDetail(r *http.Request, fallback string) string {
	if r.Header.Get("X-API-Key") != "" {
		return Detail401UseAuthBearerHint
	}
	return fallback
}

// Auth validates JWT token and injects claims into the request context.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			logger.Get().Info().
				Str("request_id", GetRequestID(r.Context())).
				Str("path", r.URL.Path).
				Msg("Missing authorization header")
			httputil.Respond401(w, r, missingAuthDetail(r, Detail401MissingAuthHeader), GetRequestID(r.Context()))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			logger.Get().Info().
				Str("request_id", GetRequestID(r.Context())).
				Str("path", r.URL.Path).
				Msg("Invalid authorization header format")
			httputil.Respond401(w, r, Detail401InvalidAuthFormat, GetRequestID(r.Context()))
			return
		}
		token := parts[1]

		claims, err := jwt.Validate(token)
		if err != nil {
			logger.Get().Info().
				Err(err).
				Str("request_id", GetRequestID(r.Context())).
				Str("path", r.URL.Path).
				Msg("JWT validation failed")
			httputil.Respond401(w, r, Detail401InvalidOrExpiredToken, GetRequestID(r.Context()))
			return
		}

		if claims == nil {
			logger.Get().Error().
				Str("request_id", GetRequestID(r.Context())).
				Str("path", r.URL.Path).
				Msg("Validate returned nil claims without error")
			httputil.Respond401(w, r, Detail401InvalidOrExpiredToken, GetRequestID(r.Context()))
			return
		}

		ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID extracts the request ID from the context.
func GetRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value(requestIDKey).(string); ok {
		return reqID
	}
	return ""
}

// GetUserClaims extracts JWT claims from the request context.
func GetUserClaims(r *http.Request) *jwt.Claims {
	if claims, ok := r.Context().Value(UserClaimsKey).(*jwt.Claims); ok {
		return claims
	}
	return nil
}

// WithUserClaimsForTest attaches session user claims to the context.
// Exported for tests only.
func WithUserClaimsForTest(ctx context.Context, c *jwt.Claims) context.Context {
	return context.WithValue(ctx, UserClaimsKey, c)
}

// SentryContext enriches Sentry scope with request ID and user info.
// Should be placed AFTER RequestID and Auth middlewares in the chain.
func SentryContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetTag("request_id", GetRequestID(r.Context()))

			if claims := GetUserClaims(r); claims != nil {
				hub.Scope().SetUser(sentry.User{
					ID:    strconv.Itoa(claims.UserID),
					Email: claims.Email,
				})
			}
		}
		next.ServeHTTP(w, r)
	})
}

var (
	ulidMu      sync.Mutex
	ulidEntropy io.Reader = ulid.Monotonic(rand.Reader, 0)
)

func generateRequestID() string {
	ulidMu.Lock()
	defer ulidMu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy).String()
}
