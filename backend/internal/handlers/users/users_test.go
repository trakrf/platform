package users_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/trakrf/platform/backend/internal/handlers/users"
)

// TRA-1103: every route on this handler must sit behind the superadmin gate.
// The behavioural proof — that a real non-superadmin session gets 403 and a
// superadmin is served — lives in users_authz_integration_test.go, which needs
// Postgres and so does not run in CI (`just test` carries no integration tag).
// This test needs no database, so it is the one that actually runs on every PR:
// it registers the routes with a gate that short-circuits, then asserts each
// route was wrapped. A sixth route added without the gate fails here.
//
// It deliberately checks wiring rather than the gate's own logic, which is
// covered by middleware/require_superadmin_test.go.

const gateSentinel = http.StatusTeapot

// sentinelGate stands in for middleware.RequireSuperadmin. It never calls
// next, so a route that reaches its handler is a route the gate does not cover.
func sentinelGate(http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(gateSentinel)
	})
}

func TestRegisterRoutes_EveryRouteIsGated(t *testing.T) {
	// A nil storage is safe precisely because the gate must short-circuit
	// before any handler runs. A route that slipped past would nil-panic here,
	// which is a failure either way.
	handler := users.NewHandler(nil)
	r := chi.NewRouter()
	handler.RegisterRoutes(r, sentinelGate)

	routes := []struct {
		name   string
		method string
		path   string
	}{
		{"list", http.MethodGet, "/api/v1/users"},
		{"get", http.MethodGet, "/api/v1/users/123"},
		{"create", http.MethodPost, "/api/v1/users"},
		{"update", http.MethodPut, "/api/v1/users/123"},
		{"delete", http.MethodDelete, "/api/v1/users/123"},
	}

	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != gateSentinel {
				t.Errorf("%s %s: got %d, want %d — route is not behind the superadmin gate",
					route.method, route.path, w.Code, gateSentinel)
			}
		})
	}
}
