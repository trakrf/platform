package main

import (
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	"github.com/trakrf/platform/backend/internal/storage"
)

func setupTestRouter(t *testing.T) *chi.Mux {
	t.Helper()

	store := &storage.Storage{}
	authSvc := authservice.NewService(nil, store, nil)
	orgsSvc := orgsservice.NewService(nil, store, nil)

	authHandler := authhandler.NewHandler(authSvc)
	orgsHandler := orgshandler.NewHandler(store, orgsSvc)
	usersHandler := usershandler.NewHandler(store)
	assetsHandler := assetshandler.NewHandler(store)
	locationsHandler := locationshandler.NewHandler(store)
	healthHandler := healthhandler.NewHandler(nil, "test", time.Now())
	frontendHandler := frontendhandler.NewHandler(frontendFS, "frontend/dist")

	return setupRouter(authHandler, orgsHandler, usersHandler, assetsHandler, locationsHandler, healthHandler, frontendHandler, store)
}

func TestRouterSetup(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("setupRouter panicked: %v", r)
		}
	}()

	r := setupTestRouter(t)

	if r == nil {
		t.Fatal("setupRouter returned nil")
	}
}

func TestRouterRegistration(t *testing.T) {
	r := setupTestRouter(t)

	tests := []struct {
		method string
		path   string
	}{
		{"GET", "/healthz"},
		{"GET", "/readyz"},
		{"GET", "/health"},
		{"POST", "/api/v1/auth/signup"},
		{"POST", "/api/v1/auth/login"},
		{"POST", "/api/v1/auth/forgot-password"},
		{"POST", "/api/v1/auth/reset-password"},
		{"POST", "/api/v1/auth/accept-invite"},
		{"GET", "/api/v1/orgs"},
		{"POST", "/api/v1/orgs"},
		{"GET", "/api/v1/orgs/1/members"},
		{"PUT", "/api/v1/orgs/1/members/2"},
		{"DELETE", "/api/v1/orgs/1/members/2"},
		{"GET", "/api/v1/orgs/1/invitations"},
		{"POST", "/api/v1/orgs/1/invitations"},
		{"DELETE", "/api/v1/orgs/1/invitations/5"},
		{"POST", "/api/v1/orgs/1/invitations/5/resend"},
		{"GET", "/api/v1/users/me"},
		{"POST", "/api/v1/users/me/current-org"},
		{"GET", "/api/v1/users"},
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
