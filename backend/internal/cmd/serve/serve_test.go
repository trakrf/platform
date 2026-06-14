package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/alarm"
	"github.com/trakrf/platform/backend/internal/alarm/shelly"
	"github.com/trakrf/platform/backend/internal/buildinfo"
	antennapowerhandler "github.com/trakrf/platform/backend/internal/handlers/antennapower"
	assetshandler "github.com/trakrf/platform/backend/internal/handlers/assets"
	authhandler "github.com/trakrf/platform/backend/internal/handlers/auth"
	frontendhandler "github.com/trakrf/platform/backend/internal/handlers/frontend"
	healthhandler "github.com/trakrf/platform/backend/internal/handlers/health"
	inventoryhandler "github.com/trakrf/platform/backend/internal/handlers/inventory"
	locationshandler "github.com/trakrf/platform/backend/internal/handlers/locations"
	lookuphandler "github.com/trakrf/platform/backend/internal/handlers/lookup"
	musteringhandler "github.com/trakrf/platform/backend/internal/handlers/mustering"
	orgshandler "github.com/trakrf/platform/backend/internal/handlers/orgs"
	outputdeviceshandler "github.com/trakrf/platform/backend/internal/handlers/outputdevices"
	readstreamhandler "github.com/trakrf/platform/backend/internal/handlers/readstream"
	reportshandler "github.com/trakrf/platform/backend/internal/handlers/reports"
	scandeviceshandler "github.com/trakrf/platform/backend/internal/handlers/scandevices"
	scanpointshandler "github.com/trakrf/platform/backend/internal/handlers/scanpoints"
	testhandler "github.com/trakrf/platform/backend/internal/handlers/testhandler"
	usershandler "github.com/trakrf/platform/backend/internal/handlers/users"
	"github.com/trakrf/platform/backend/internal/ingest"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/internal/mustering"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	orgsservice "github.com/trakrf/platform/backend/internal/services/orgs"
	readstreamsvc "github.com/trakrf/platform/backend/internal/services/readstream"
	"github.com/trakrf/platform/backend/internal/storage"
)

func setupTestRouter(t *testing.T) *chi.Mux {
	t.Helper()

	store := &storage.Storage{}
	authSvc := authservice.NewService(nil, store, nil)
	orgsSvc := orgsservice.NewService(nil, store, nil)

	authHandler := authhandler.NewHandler(authSvc, store)
	orgsHandler := orgshandler.NewHandler(store, orgsSvc, authSvc)
	usersHandler := usershandler.NewHandler(store)
	assetsHandler := assetshandler.NewHandler(store)
	locationsHandler := locationshandler.NewHandler(store)
	inventoryHandler := inventoryhandler.NewHandler(store)
	reportsHandler := reportshandler.NewHandler(store)
	scanDevicesHandler := scandeviceshandler.NewHandler(store, nil)
	scanPointsHandler := scanpointshandler.NewHandler(store)
	outputDevicesHandler := outputdeviceshandler.NewHandler(store, alarm.NewDispatcher(shelly.New(0), nil), 0)
	antennaPowerHandler := antennapowerhandler.NewHandler(store, nil)
	lookupHandler := lookuphandler.NewHandler(store)
	healthHandler := healthhandler.NewHandler(nil, buildinfo.Info{Version: "test"}, time.Now())
	frontendHandler := frontendhandler.NewHandler(fstest.MapFS{}, "frontend/dist", "")
	readstreamHandler := readstreamhandler.NewHandler(readstreamsvc.New())
	musterBC := mustering.NewBroadcaster()
	musterEngine := mustering.NewEngine(store, musterBC, logger.Get())
	musteringHandler := musteringhandler.NewHandler(musterEngine, musterBC, store, ingest.MultiEvaluator{musterEngine}, nil)
	testHandler := testhandler.NewHandler(store)

	return setupRouter(authHandler, orgsHandler, usersHandler, assetsHandler, locationsHandler, inventoryHandler, reportsHandler, scanDevicesHandler, scanPointsHandler, outputDevicesHandler, antennaPowerHandler, lookupHandler, healthHandler, frontendHandler, readstreamHandler, musteringHandler, testHandler, store)
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
		{"GET", "/api/v1/reads/stream"},
		{"GET", "/api/v1/mustering/stream"},
		{"GET", "/api/v1/mustering/status"},
		{"POST", "/api/v1/mustering/events"},
		{"POST", "/api/v1/mustering/events/1/all-clear"},
		{"PATCH", "/api/v1/mustering/events/1/entries/2"},
		{"POST", "/api/v1/mustering/simulate"},
		{"POST", "/api/v1/mustering/seed"},
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

// TRA-693 / BB30 §2.3: /api/v1/openapi.{json,yaml} redirects to the canonical
// /api/openapi.{json,yaml} variant. Permanent (301) so caching proxies can
// rewrite indefinitely.
func TestPublicOpenAPISpec_V1Path_RedirectsToCanonical_JSON(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /api/v1/openapi.json = %d, want 301; body: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/openapi.json" {
		t.Fatalf("Location = %q, want /api/openapi.json", loc)
	}
}

func TestPublicOpenAPISpec_V1Path_RedirectsToCanonical_YAML(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /api/v1/openapi.yaml = %d, want 301; body: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/openapi.yaml" {
		t.Fatalf("Location = %q, want /api/openapi.yaml", loc)
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

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /openapi.json = %d, want 301; body: %s", rec.Code, rec.Body.String())
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

	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET /openapi.yaml = %d, want 301; body: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/openapi.yaml" {
		t.Fatalf("Location = %q, want /api/openapi.yaml", loc)
	}
}

func TestHeadRequestMatches_OpenAPISpec(t *testing.T) {
	r := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodHead, "/api/openapi.json", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD /api/openapi.json = %d, want 200; body: %s", rec.Code, rec.Body.String())
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

// TestRouter_MovedLocationsCurrent_Returns404 — TRA-658 BB25. The
// retired /api/v1/locations/current must return 404 for every method
// against the full production router, not fall through to
// /api/v1/locations/{location_id} and surface 401 (auth runs before the
// sibling resolves to "current is not a valid id"). The 404 body must
// point clients at /api/v1/reports/asset-locations.
func TestRouter_MovedLocationsCurrent_Returns404(t *testing.T) {
	r := setupTestRouter(t)

	for _, method := range []string{
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
	} {
		t.Run(method+" /api/v1/locations/current", func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/locations/current", nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s /api/v1/locations/current = %d, want 404; body: %s",
					method, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "/api/v1/reports/asset-locations") {
				t.Errorf("moved-path 404 must point clients at the replacement; body: %s",
					rec.Body.String())
			}
		})
	}
}

// TestRouter_AuditedStatic_405WithCorrectAllow — TRA-600 audit. The
// audited static paths (orgs/me, users/me, reports/asset-locations,
// assets/bulk, …) must emit 405 with an Allow header that reflects only
// the static path's actual methods, never the sibling /{id}'s.
func TestRouter_AuditedStatic_405WithCorrectAllow(t *testing.T) {
	r := setupTestRouter(t)

	cases := []struct {
		path      string
		wrongVerb string
		wantAllow string
	}{
		// TRA-599: /orgs/me four-method coverage (the ticket's
		// repro spans POST/PATCH/DELETE/PUT — pin all four).
		{"/api/v1/orgs/me", http.MethodPost, "GET, HEAD"},
		{"/api/v1/orgs/me", http.MethodPatch, "GET, HEAD"},
		{"/api/v1/orgs/me", http.MethodPut, "GET, HEAD"},
		{"/api/v1/orgs/me", http.MethodDelete, "GET, HEAD"},
		{"/api/v1/users/me", http.MethodPut, "GET, HEAD"},
		{"/api/v1/users/me", http.MethodDelete, "GET, HEAD"},
		{"/api/v1/users/me/current-org", http.MethodGet, "POST"},
		{"/api/v1/users/me/current-org", http.MethodDelete, "POST"},
		{"/api/v1/reports/asset-locations", http.MethodPut, "GET, HEAD"},
		{"/api/v1/reports/asset-locations", http.MethodDelete, "GET, HEAD"},
		{"/api/v1/assets/bulk", http.MethodGet, "POST"},
		{"/api/v1/assets/bulk", http.MethodDelete, "POST"},
		{"/api/v1/assets/bulk/abc123", http.MethodPut, "GET, HEAD"},
		{"/api/v1/assets/bulk/abc123", http.MethodDelete, "GET, HEAD"},

		// TRA-604: parametric /orgs/{id} sub-tree must 405 with the
		// real Allow set on wrong methods. Previously these emitted 401
		// because middleware.Auth ran before chi's MethodNotAllowed
		// determination on the r.Route() sub-router mount.
		{"/api/v1/orgs/abc", http.MethodPost, "GET, HEAD, PUT, DELETE"},
		{"/api/v1/orgs/abc", http.MethodPatch, "GET, HEAD, PUT, DELETE"},
		{"/api/v1/orgs/abc/members", http.MethodPost, "GET, HEAD"},
		{"/api/v1/orgs/abc/members", http.MethodPut, "GET, HEAD"},
		{"/api/v1/orgs/abc/members/2", http.MethodGet, "PUT, DELETE"},
		{"/api/v1/orgs/abc/members/2", http.MethodPatch, "PUT, DELETE"},
		{"/api/v1/orgs/abc/invitations", http.MethodPut, "GET, HEAD, POST"},
		{"/api/v1/orgs/abc/invitations", http.MethodDelete, "GET, HEAD, POST"},
		{"/api/v1/orgs/abc/invitations/5", http.MethodGet, "DELETE"},
		{"/api/v1/orgs/abc/invitations/5", http.MethodPost, "DELETE"},
		{"/api/v1/orgs/abc/invitations/5/resend", http.MethodGet, "POST"},
		{"/api/v1/orgs/abc/invitations/5/resend", http.MethodDelete, "POST"},
		{"/api/v1/orgs/abc/api-keys", http.MethodPut, "GET, HEAD, POST"},
		{"/api/v1/orgs/abc/api-keys", http.MethodPatch, "GET, HEAD, POST"},
		{"/api/v1/orgs/abc/api-keys/key123", http.MethodGet, "DELETE"},
		{"/api/v1/orgs/abc/api-keys/key123", http.MethodPut, "DELETE"},
		{"/api/v1/orgs/abc/api-keys/by-jti/abc", http.MethodGet, "DELETE"},
		{"/api/v1/orgs/abc/api-keys/by-jti/abc", http.MethodPut, "DELETE"},

		// TRA-605: write-only paths must 405 (not 404 via the catchall)
		// on GET, with Allow reflecting the real method set — not a
		// phantom GET, HEAD synthesized from the catchall.
		{"/api/v1/assets/abc/tags", http.MethodGet, "POST"},
		{"/api/v1/assets/abc/tags", http.MethodPatch, "POST"},
		{"/api/v1/assets/abc/tags/tag1", http.MethodGet, "DELETE"},
		{"/api/v1/assets/abc/tags/tag1", http.MethodPut, "DELETE"},
		{"/api/v1/locations/abc/tags", http.MethodGet, "POST"},
		{"/api/v1/locations/abc/tags", http.MethodPatch, "POST"},
		{"/api/v1/locations/abc/tags/tag1", http.MethodGet, "DELETE"},
		{"/api/v1/locations/abc/tags/tag1", http.MethodPut, "DELETE"},
		{"/api/v1/inventory/save", http.MethodGet, "POST"},
		{"/api/v1/inventory/save", http.MethodPatch, "POST"},
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
			if wwwAuth := rec.Header().Get("WWW-Authenticate"); wwwAuth != "" {
				t.Errorf("%s %s WWW-Authenticate header leaked on 405: %q",
					tc.wrongVerb, tc.path, wwwAuth)
			}
		})
	}
}

// TestRouter_OrgsSubtree_RegisteredMethodsStill401 — TRA-604 acceptance.
// Flattening r.Route() must NOT regress the auth chain on registered
// methods: GET/PUT/DELETE on /api/v1/orgs/{id} and friends still 401
// without credentials. The fix only affects wrong-method paths.
func TestRouter_OrgsSubtree_RegisteredMethodsStill401(t *testing.T) {
	r := setupTestRouter(t)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/orgs/abc"},
		{http.MethodPut, "/api/v1/orgs/abc"},
		{http.MethodDelete, "/api/v1/orgs/abc"},
		{http.MethodGet, "/api/v1/orgs/abc/members"},
		{http.MethodPut, "/api/v1/orgs/abc/members/2"},
		{http.MethodDelete, "/api/v1/orgs/abc/members/2"},
		{http.MethodGet, "/api/v1/orgs/abc/invitations"},
		{http.MethodPost, "/api/v1/orgs/abc/invitations"},
		{http.MethodDelete, "/api/v1/orgs/abc/invitations/5"},
		{http.MethodPost, "/api/v1/orgs/abc/invitations/5/resend"},
		{http.MethodGet, "/api/v1/orgs/abc/api-keys"},
		{http.MethodPost, "/api/v1/orgs/abc/api-keys"},
		{http.MethodDelete, "/api/v1/orgs/abc/api-keys/key123"},
		{http.MethodDelete, "/api/v1/orgs/abc/api-keys/by-jti/abc"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s = %d, want 401; body: %s",
					tc.method, tc.path, rec.Code, rec.Body.String())
			}
		})
	}
}

// TestRouter_UnknownAPIPath_Returns404 — TRA-605. Truly-unknown /api/*
// paths still return 404 via the catchall. This is the success case the
// catchall was originally introduced for; the TRA-605 fix must preserve
// it while distinguishing wrong-method on a registered path (405).
func TestRouter_UnknownAPIPath_Returns404(t *testing.T) {
	r := setupTestRouter(t)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/totally-not-a-route"},
		{http.MethodPost, "/api/v1/also-fake"},
		{http.MethodPatch, "/api/v2/anything"},
		{http.MethodDelete, "/api/v1/foo/bar/baz"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s %s = %d, want 404; body: %s",
					tc.method, tc.path, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "Unknown API route") {
				t.Errorf("404 body missing Unknown-API marker: %s", rec.Body.String())
			}
		})
	}
}
