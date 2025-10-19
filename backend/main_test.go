package main

import (
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestRouterSetup verifies router can be created without panic
// This catches route registration errors (e.g., invalid wildcard patterns)
// before they reach production
func TestRouterSetup(t *testing.T) {
	// This will panic if route registration fails
	// (e.g., "wildcard '*' must be the last value in a route")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("setupRouter panicked: %v", r)
		}
	}()

	r := setupRouter()

	if r == nil {
		t.Fatal("setupRouter returned nil")
	}
}

// TestRouterRegistration verifies critical routes are registered
func TestRouterRegistration(t *testing.T) {
	r := setupRouter()

	tests := []struct {
		method string
		path   string
	}{
		// Health endpoints
		{"GET", "/healthz"},
		{"GET", "/readyz"},
		{"GET", "/health"},

		// Auth endpoints
		{"POST", "/api/v1/auth/signup"},
		{"POST", "/api/v1/auth/login"},

		// Protected endpoints (will fail auth, but route should exist)
		{"GET", "/api/v1/accounts"},
		{"GET", "/api/v1/users"},

		// Frontend routes
		{"GET", "/assets/index.js"},
		{"GET", "/favicon.ico"},
		{"GET", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rctx := chi.NewRouteContext()

			if !r.Match(rctx, tt.method, tt.path) {
				t.Errorf("Route not found: %s %s", tt.method, tt.path)
			}
		})
	}
}
