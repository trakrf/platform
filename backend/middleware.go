package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
)

type contextKey string

const requestIDKey contextKey = "requestID"
const UserClaimsKey contextKey = "user_claims"

// requestIDMiddleware generates or extracts request ID
func requestIDMiddleware(next http.Handler) http.Handler {
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

func generateRequestID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// recoveryMiddleware catches panics and returns 500
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				requestID := getRequestID(r.Context())
				slog.Error("Panic recovered",
					"error", err,
					"request_id", requestID,
					"path", r.URL.Path,
					"method", r.Method)

				writeJSONError(w, r, http.StatusInternalServerError,
					ErrInternal, "Internal server error", "")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware handles CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Make origin configurable via BACKEND_CORS_ORIGIN env var
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
		w.Header().Set("Access-Control-Max-Age", "3600")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// contentTypeMiddleware enforces JSON for write operations
func contentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" || r.Method == "PUT" {
			ct := r.Header.Get("Content-Type")
			if ct != "application/json" && ct != "application/json; charset=utf-8" && ct != "" {
				writeJSONError(w, r, http.StatusUnsupportedMediaType,
					ErrBadRequest, "Content-Type must be application/json", "")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func getRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value(requestIDKey).(string); ok {
		return reqID
	}
	return ""
}

// authMiddleware validates JWT token and injects claims into context
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			slog.Info("Missing authorization header",
				"request_id", getRequestID(r.Context()),
				"path", r.URL.Path)
			writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized,
				"Missing authorization header", "")
			return
		}

		// 2. Parse "Bearer {token}" format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			slog.Info("Invalid authorization header format",
				"request_id", getRequestID(r.Context()),
				"path", r.URL.Path)
			writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized,
				"Invalid authorization header format", "")
			return
		}
		token := parts[1]

		// 3. Validate JWT using Phase 5A ValidateJWT()
		claims, err := ValidateJWT(token)
		if err != nil {
			slog.Info("JWT validation failed",
				"error", err,
				"request_id", getRequestID(r.Context()),
				"path", r.URL.Path)
			writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized,
				"Invalid or expired token", "")
			return
		}

		// 4. Defensive nil check (idiomatic Go)
		if claims == nil {
			slog.Error("ValidateJWT returned nil claims without error",
				"request_id", getRequestID(r.Context()),
				"path", r.URL.Path)
			writeJSONError(w, r, http.StatusUnauthorized, ErrUnauthorized,
				"Invalid or expired token", "")
			return
		}

		// 5. Inject claims into context and proceed
		ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserClaims extracts JWT claims from request context
// Returns nil if claims not found (handlers should validate defensively)
func GetUserClaims(r *http.Request) *JWTClaims {
	if claims, ok := r.Context().Value(UserClaimsKey).(*JWTClaims); ok {
		return claims
	}
	return nil
}
