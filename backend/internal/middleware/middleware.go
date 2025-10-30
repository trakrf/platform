package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"strings"

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
				slog.Error("Panic recovered",
					"error", err,
					"request_id", requestID,
					"path", r.URL.Path,
					"method", r.Method)

				httputil.WriteJSONError(w, r, http.StatusInternalServerError,
					errors.ErrInternal, "Internal server error", "", requestID)
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
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
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
// - multipart/form-data (file uploads)
// - empty Content-Type (legacy compatibility)
func ContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
			ct := r.Header.Get("Content-Type")

			// Empty Content-Type is allowed for backwards compatibility
			if ct == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check against allowed content types
			// Note: multipart/form-data includes boundary parameter
			isAllowed := ct == "application/json" ||
				ct == "application/json; charset=utf-8" ||
				strings.HasPrefix(ct, "multipart/form-data")

			if !isAllowed {
				httputil.WriteJSONError(w, r, http.StatusUnsupportedMediaType,
					errors.ErrBadRequest,
					"Content-Type must be application/json or multipart/form-data",
					"",
					GetRequestID(r.Context()))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Auth validates JWT token and injects claims into the request context.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			slog.Info("Missing authorization header",
				"request_id", GetRequestID(r.Context()),
				"path", r.URL.Path)
			httputil.WriteJSONError(w, r, http.StatusUnauthorized, errors.ErrUnauthorized,
				"Missing authorization header", "", GetRequestID(r.Context()))
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			slog.Info("Invalid authorization header format",
				"request_id", GetRequestID(r.Context()),
				"path", r.URL.Path)
			httputil.WriteJSONError(w, r, http.StatusUnauthorized, errors.ErrUnauthorized,
				"Invalid authorization header format", "", GetRequestID(r.Context()))
			return
		}
		token := parts[1]

		claims, err := jwt.Validate(token)
		if err != nil {
			slog.Info("JWT validation failed",
				"error", err,
				"request_id", GetRequestID(r.Context()),
				"path", r.URL.Path)
			httputil.WriteJSONError(w, r, http.StatusUnauthorized, errors.ErrUnauthorized,
				"Invalid or expired token", "", GetRequestID(r.Context()))
			return
		}

		if claims == nil {
			slog.Error("Validate returned nil claims without error",
				"request_id", GetRequestID(r.Context()),
				"path", r.URL.Path)
			httputil.WriteJSONError(w, r, http.StatusUnauthorized, errors.ErrUnauthorized,
				"Invalid or expired token", "", GetRequestID(r.Context()))
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

func generateRequestID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
