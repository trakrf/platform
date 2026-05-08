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
		{"GET", "/api/openapi.json"},
		{"GET", "/api/openapi.yaml"},
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

// TRA-479: the documented integrator URL is /api/openapi.{json,yaml} (no v1
// segment). Must serve the spec directly, not redirect — external codegen
// tools like openapi-generator-cli plug this URL in as-is.
func TestPublicOpenAPISpec_ServedAt_UnversionedPath_JSON(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/openapi.json = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), `"openapi"`) {
		t.Fatalf("body does not contain \"openapi\" key: %s", rec.Body.String()[:min(200, len(rec.Body.String()))])
	}
}

func TestPublicOpenAPISpec_ServedAt_UnversionedPath_YAML(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/openapi.yaml = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Fatalf("Content-Type = %q, want application/yaml", ct)
	}
	if !strings.Contains(rec.Body.String(), "openapi:") {
		t.Fatalf("body does not contain \"openapi:\" key: %s", rec.Body.String()[:min(200, len(rec.Body.String()))])
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
	if loc := rec.Header().Get("Location"); loc != "/api/openapi.json" {
		t.Fatalf("Location = %q, want /api/openapi.json", loc)
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
	if loc := rec.Header().Get("Location"); loc != "/api/openapi.yaml" {
		t.Fatalf("Location = %q, want /api/openapi.yaml", loc)
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

// TestRouter_RetiredLookupPath_Returns404 — TRA-600. The retired
// /assets/lookup and /locations/lookup paths must return 404 for every
// method against the full production router, not fall through to /{id}
// and return 400 invalid-id or 405 with a misleading Allow header.
func TestRouter_RetiredLookupPath_Returns404(t *testing.T) {
	r := setupTestRouter(t)

	for _, path := range []string{"/api/v1/assets/lookup", "/api/v1/locations/lookup"} {
		for _, method := range []string{
			http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
		} {
			t.Run(method+" "+path, func(t *testing.T) {
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
				if rec.Code != http.StatusNotFound {
					t.Fatalf("%s %s = %d, want 404; body: %s",
						method, path, rec.Code, rec.Body.String())
				}
				if !strings.Contains(rec.Body.String(), "external_key=") {
					t.Errorf("retired-path 404 must point clients at the replacement; body: %s",
						rec.Body.String())
				}
			})
		}
	}
}

// TestRouter_AuditedStatic_405WithCorrectAllow — TRA-600 audit. The
// audited static paths (orgs/me, users/me, locations/current,
// assets/bulk, …) must emit 405 with an Allow header that reflects only
// the static path's actual methods, never the sibling /{id}'s.
func TestRouter_AuditedStatic_405WithCorrectAllow(t *testing.T) {
	r := setupTestRouter(t)

	cases := []struct {
		path      string
		wrongVerb string
		wantAllow string
	}{
		{"/api/v1/orgs/me", http.MethodPut, "GET, HEAD"},
		{"/api/v1/orgs/me", http.MethodDelete, "GET, HEAD"},
		{"/api/v1/users/me", http.MethodPut, "GET, HEAD"},
		{"/api/v1/users/me", http.MethodDelete, "GET, HEAD"},
		{"/api/v1/users/me/current-org", http.MethodGet, "POST"},
		{"/api/v1/users/me/current-org", http.MethodDelete, "POST"},
		{"/api/v1/locations/current", http.MethodPut, "GET, HEAD"},
		{"/api/v1/locations/current", http.MethodDelete, "GET, HEAD"},
		{"/api/v1/assets/bulk", http.MethodGet, "POST"},
		{"/api/v1/assets/bulk", http.MethodDelete, "POST"},
		{"/api/v1/assets/bulk/abc123", http.MethodPut, "GET, HEAD"},
		{"/api/v1/assets/bulk/abc123", http.MethodDelete, "GET, HEAD"},
	}

	for _, tc := range cases {
		t.Run(tc.wrongVerb+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(tc.wrongVerb, tc.path, nil))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s = %d, want 405; body: %s",
					tc.wrongVerb, tc.path, rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Allow"); got != tc.wantAllow {
				t.Errorf("%s %s Allow = %q, want %q",
					tc.wrongVerb, tc.path, got, tc.wantAllow)
			}
		})
	}
}
