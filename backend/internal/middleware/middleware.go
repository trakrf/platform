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
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := os.Getenv("BACKEND_CORS_ORIGIN")
		if origin == "" {
			origin = "*"
		}

		if origin != "disabled" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "3600")
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ContentType enforces allowed Content-Type headers for write operations.
// Allows:
// - application/json (standard API requests)
// - application/merge-patch+json (RFC 7396 — PATCH operations)
// - multipart/form-data (internal bulk-CSV upload)
// - empty Content-Type (legacy compatibility)
//
// PATCH operations follow RFC 7396 strict merge-patch semantics — the
// public spec declares only application/merge-patch+json on PATCH. That
// strictness lives in RequireMergePatchCT, attached per-route on the two
// PATCH endpoints, so undeclared-PATCH probes against POST-only paths
// (e.g. /assets/{id}/tags) get chi's 405 instead of a 415 from this
// middleware running before routing.
func ContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
			ct := r.Header.Get("Content-Type")

			// Empty Content-Type is allowed for backwards compatibility
			if ct == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Note: multipart/form-data includes boundary parameter
			isAllowed := ct == "application/json" ||
				ct == "application/json; charset=utf-8" ||
				ct == "application/merge-patch+json" ||
				ct == "application/merge-patch+json; charset=utf-8" ||
				strings.HasPrefix(ct, "multipart/form-data")

			if !isAllowed {
				httputil.Respond415(w, r, GetRequestID(r.Context()))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequireMergePatchCT is a per-route middleware that enforces the
// PATCH-strict content-type. Attach to PATCH handlers so the public spec's
// declared `application/merge-patch+json` is the only accepted CT on those
// operations (RFC 7396; BB28 W2/S4). Empty Content-Type is allowed for
// backwards compatibility, matching the global ContentType policy.
//
// This is per-route rather than global so PATCH probes against paths
// without a registered PATCH handler (POST-only /tags, /rename subpaths)
// produce chi's natural 405 instead of being intercepted with a 415.
func RequireMergePatchCT(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "" && ct != "application/merge-patch+json" && ct != "application/merge-patch+json; charset=utf-8" {
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
		return "Use Authorization: Bearer <token>"
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
			httputil.Respond401(w, r, missingAuthDetail(r, "Authorization header is required"), GetRequestID(r.Context()))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			logger.Get().Info().
				Str("request_id", GetRequestID(r.Context())).
				Str("path", r.URL.Path).
				Msg("Invalid authorization header format")
			httputil.Respond401(w, r, "Authorization header must be Bearer <token>", GetRequestID(r.Context()))
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
			httputil.Respond401(w, r, "Bearer token is invalid or expired", GetRequestID(r.Context()))
			return
		}

		if claims == nil {
			logger.Get().Error().
				Str("request_id", GetRequestID(r.Context())).
				Str("path", r.URL.Path).
				Msg("Validate returned nil claims without error")
			httputil.Respond401(w, r, "Bearer token is invalid or expired", GetRequestID(r.Context()))
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
