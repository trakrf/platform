package main

import (
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	orgusershandler "github.com/trakrf/platform/backend/internal/handlers/org_users"
	organizationshandler "github.com/trakrf/platform/backend/internal/handlers/organizations"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/storage"
)

func setupTestRouter(t *testing.T) *chi.Mux {
	t.Helper()

	store := &storage.Storage{}
	authSvc := authservice.NewService(nil, store)

	authHandler := authhandler.NewHandler(authSvc)
	organizationsHandler := organizationshandler.NewHandler(store)
	usersHandler := usershandler.NewHandler(store)
	orgUsersHandler := orgusershandler.NewHandler(store)
	healthHandler := healthhandler.NewHandler(nil, "test", time.Now())
	frontendHandler := frontendhandler.NewHandler(frontendFS, "frontend/dist")

	return setupRouter(authHandler, organizationsHandler, usersHandler, orgUsersHandler, healthHandler, frontendHandler)
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
		{"GET", "/api/v1/organizations"},
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
