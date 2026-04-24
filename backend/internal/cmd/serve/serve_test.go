package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/buildinfo"
	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	inventoryhandler "github.com/trakrf/platform/backend/internal/handlers/inventory"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	lookuphandler "github.com/trakrf/platform/backend/internal/handlers/lookup"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	reportshandler "github.com/trakrf/platform/backend/internal/handlers/reports"
	testhandler "github.com/trakrf/platform/backend/internal/handlers/testhandler"
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
	inventoryHandler := inventoryhandler.NewHandler(store)
	reportsHandler := reportshandler.NewHandler(store)
	lookupHandler := lookuphandler.NewHandler(store)
	healthHandler := healthhandler.NewHandler(nil, buildinfo.Info{Version: "test"}, time.Now())
	frontendHandler := frontendhandler.NewHandler(fstest.MapFS{}, "frontend/dist")
	testHandler := testhandler.NewHandler(store)

	return setupRouter(authHandler, orgsHandler, usersHandler, assetsHandler, locationsHandler, inventoryHandler, reportsHandler, lookupHandler, healthHandler, frontendHandler, testHandler, store)
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
		{"GET", "/metrics"},
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
		{"GET", "/version.json"},
		{"GET", "/"},
		{"GET", "/api/v1/openapi.json"},
		{"GET", "/api/v1/openapi.yaml"},
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

func TestPublicOpenAPISpec_ServedAt_V1Path(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/openapi.json = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), `"openapi"`) {
		t.Fatalf("body does not contain \"openapi\" key: %s", rec.Body.String()[:min(200, len(rec.Body.String()))])
	}
}

func TestMetricsEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics: got status %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "# HELP ") {
		t.Errorf("GET /metrics: response missing Prometheus '# HELP' marker; got first 200 bytes: %q", body[:min(200, len(body))])
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Errorf("GET /metrics: response missing default Go runtime metric 'go_goroutines'")
	}
}

func TestOpenAPISpec_RootRedirect_JSON(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("GET /openapi.json = %d, want 302; body: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/v1/openapi.json" {
		t.Fatalf("Location = %q, want /api/v1/openapi.json", loc)
	}
}

func TestOpenAPISpec_RootRedirect_YAML(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("GET /openapi.yaml = %d, want 302; body: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/v1/openapi.yaml" {
		t.Fatalf("Location = %q, want /api/v1/openapi.yaml", loc)
	}
}

func TestHeadRequestMatches_OpenAPISpec(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodHead, "/api/v1/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD /api/v1/openapi.json = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
}
