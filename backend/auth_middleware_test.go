package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestAuthMiddleware_MissingToken tests 401 without Authorization header
func TestAuthMiddleware_MissingToken(t *testing.T) {
	// Setup: Create chi router with auth middleware protecting test endpoint
	r := chi.NewRouter()
	r.Use(requestIDMiddleware) // Need request_id for error responses
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
			// This should never run - middleware should block
			w.WriteHeader(http.StatusOK)
		})
	})

	// Test: Request without Authorization header
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Assert: 401 Unauthorized
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Assert: Response body contains error message
	body := w.Body.String()
	if !contains(body, "Missing authorization header") {
		t.Errorf("body should contain 'Missing authorization header', got: %s", body)
	}
}

// TestAuthMiddleware_InvalidToken tests 401 with malformed token
func TestAuthMiddleware_InvalidToken(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		wantError  string
	}{
		{
			name:       "malformed bearer format",
			authHeader: "InvalidFormat",
			wantError:  "Invalid authorization header format",
		},
		{
			name:       "invalid JWT token",
			authHeader: "Bearer invalid-token-string",
			wantError:  "Invalid or expired token",
		},
		{
			name:       "missing bearer prefix",
			authHeader: "just-a-token",
			wantError:  "Invalid authorization header format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup router
			r := chi.NewRouter()
			r.Use(requestIDMiddleware)
			r.Group(func(r chi.Router) {
				r.Use(authMiddleware)
				r.Get("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				})
			})

			// Test with invalid token
			req := httptest.NewRequest("GET", "/api/v1/test", nil)
			req.Header.Set("Authorization", tt.authHeader)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			// Assert 401
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
			}

			// Assert error message
			body := w.Body.String()
			if !contains(body, tt.wantError) {
				t.Errorf("body should contain %q, got: %s", tt.wantError, body)
			}
		})
	}
}

// TestAuthMiddleware_ValidToken tests 200 with valid JWT
func TestAuthMiddleware_ValidToken(t *testing.T) {
	// Generate valid JWT token
	token, err := GenerateJWT(1, "test@example.com", nil)
	if err != nil {
		t.Fatalf("GenerateJWT() failed: %v", err)
	}

	// Setup router with test endpoint that extracts claims
	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
			// Verify claims are in context
			claims := GetUserClaims(r)
			if claims == nil {
				t.Error("GetUserClaims() returned nil - claims not injected")
			}
			if claims != nil && claims.UserID != 1 {
				t.Errorf("claims.UserID = %d, want 1", claims.UserID)
			}
			if claims != nil && claims.Email != "test@example.com" {
				t.Errorf("claims.Email = %q, want %q", claims.Email, "test@example.com")
			}
			w.WriteHeader(http.StatusOK)
		})
	})

	// Test with valid token
	req := httptest.NewRequest("GET", "/api/v1/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Assert: Middleware passed, handler ran successfully
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (middleware should pass with valid token)", w.Code, http.StatusOK)
	}
}

// TestPublicEndpoints_NoAuth tests public routes work without token
func TestPublicEndpoints_NoAuth(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"health check", "GET", "/healthz"},
		{"readiness check", "GET", "/readyz"},
		{"OPTIONS for CORS", "OPTIONS", "/api/v1/auth/signup"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup full router like in main.go
			r := chi.NewRouter()
			r.Use(requestIDMiddleware)
			r.Use(corsMiddleware)

			// Health checks (public)
			r.Get("/healthz", healthzHandler)
			r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
				// Simplified readyz for test (no DB)
				w.WriteHeader(http.StatusOK)
			})

			// Auth routes (public)
			r.Post("/api/v1/auth/signup", signupHandler)

			// Test without Authorization header
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			// Assert: NOT 401 (public endpoints should work)
			if w.Code == http.StatusUnauthorized {
				t.Errorf("public endpoint returned 401 - should be accessible without auth")
			}
		})
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
