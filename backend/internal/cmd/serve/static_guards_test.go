package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

// TRA-600: register404Static and register405Static guard static-segment
// paths from chi v5's static-vs-{id} fall-through. The bug they fix:
// methods unregistered on a static path resolve to a sibling /{id} route,
// returning 400 invalid-id (when the {id} handler runs) or 405 with a
// misleading Allow header that lists the {id} route's methods.

// reproRouter wires a router that exhibits the bug without the guards.
// Used to assert the guards' effect against a known-broken baseline.
func reproRouter(useGuards bool) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		allowed := computeAllowedMethods(r, req.URL.Path)
		httputil.Respond405(w, req, allowed, middleware.GetRequestID(req.Context()))
	})

	r.Get("/api/v1/assets/{asset_id}", func(w http.ResponseWriter, req *http.Request) {
		// Stand-in for the real GetAsset handler: parses {asset_id} as an int.
		idParam := chi.URLParam(req, "asset_id")
		if idParam == "lookup" || idParam == "me" || idParam == "bulk" {
			httputil.WriteJSONError(w, req, http.StatusBadRequest, apierrors.ErrBadRequest,
				"invalid id \""+idParam+"\": must be a positive integer",
				middleware.GetRequestID(req.Context()))
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	r.Patch("/api/v1/assets/{asset_id}", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Delete("/api/v1/assets/{asset_id}", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Get("/api/v1/orgs/me", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// /orgs/{id} sibling that {id}=me would fall through to. Bug surface
	// only when this exists.
	r.Get("/api/v1/orgs/{id}", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Put("/api/v1/orgs/{id}", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Delete("/api/v1/orgs/{id}", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if useGuards {
		register404Static(r, "/api/v1/assets/lookup",
			"This endpoint has been removed. Use GET /api/v1/assets?external_key=.")
		register405Static(r, "/api/v1/orgs/me", []string{http.MethodGet})
	}
	return r
}

func runReq(t *testing.T, mux *chi.Mux, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) apierrors.ErrorResponse {
	t.Helper()
	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

// TestStaticGuards_RetiredPath_ReturnsConsistent404 — every method on a
// retired static path returns 404 with the documented detail message,
// regardless of whether the sibling /{id} route is registered for that
// method. Without register404Static the same input set returns a mix of
// 400 and 405 responses with confusing detail.
func TestStaticGuards_RetiredPath_ReturnsConsistent404(t *testing.T) {
	mux := reproRouter(true)

	for _, method := range []string{
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete,
	} {
		t.Run(method, func(t *testing.T) {
			rec := runReq(t, mux, method, "/api/v1/assets/lookup")
			require.Equal(t, http.StatusNotFound, rec.Code,
				"retired-path guard must emit 404 for every method, not fall through to /{id}")
			body := decodeError(t, rec)
			assert.Equal(t, "not_found", body.Error.Type)
			assert.Contains(t, body.Error.Detail, "external_key=",
				"retired-path 404 must point clients at the replacement endpoint")
		})
	}
}

// TestStaticGuards_RetiredPath_BugReproductionWithoutGuards — the same
// requests against the unguarded router exhibit the routing-precedence bug:
// PATCH/DELETE return 400 (matched the sibling /{id} handler) and POST/PUT
// return 405 with a misleading Allow header that names PATCH and DELETE.
// This test pins the bug so a future refactor cannot silently re-introduce
// it.
func TestStaticGuards_RetiredPath_BugReproductionWithoutGuards(t *testing.T) {
	mux := reproRouter(false)

	t.Run("PATCH falls through to {id}", func(t *testing.T) {
		rec := runReq(t, mux, http.MethodPatch, "/api/v1/assets/lookup")
		// Without guards PATCH matches /{asset_id} and runs the patch handler
		// (which returns 200 in this stand-in). The bug isn't 200 per se —
		// it's that "lookup" was never an id. With register404Static the
		// guard wins.
		assert.Equal(t, http.StatusOK, rec.Code,
			"baseline: PATCH /lookup matches the {asset_id} handler, not the static path")
	})

	t.Run("POST returns 405 with misleading Allow", func(t *testing.T) {
		rec := runReq(t, mux, http.MethodPost, "/api/v1/assets/lookup")
		require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		allow := rec.Header().Get("Allow")
		assert.Contains(t, allow, "PATCH",
			"baseline: 405 lies about supported methods because computeAllowedMethods probes /{asset_id}")
		assert.Contains(t, allow, "DELETE")
	})
}

// TestStaticGuards_LiveStaticPath_405WithCorrectAllow — a live static path
// with one supported method (GET) returns 405 for every other guarded
// method, with Allow: GET, HEAD (HEAD synthesized by the helper). The
// header reflects only the static path's real methods, never the sibling
// /{id}'s.
func TestStaticGuards_LiveStaticPath_405WithCorrectAllow(t *testing.T) {
	mux := reproRouter(true)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			rec := runReq(t, mux, method, "/api/v1/orgs/me")
			require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
			assert.Equal(t, "GET, HEAD", rec.Header().Get("Allow"),
				"Allow header must reflect /orgs/me's methods, not /orgs/{id}'s")
			body := decodeError(t, rec)
			assert.Equal(t, "method_not_allowed", body.Error.Type)
		})
	}

	t.Run("real GET still works", func(t *testing.T) {
		rec := runReq(t, mux, http.MethodGet, "/api/v1/orgs/me")
		assert.Equal(t, http.StatusOK, rec.Code,
			"register405Static must not shadow the real GET handler")
	})
}

// TestStaticGuards_HEADIsSynthesizedFromGET — the GetHead chi middleware
// rewrites HEAD→GET upstream. The guard helper synthesizes HEAD into
// the Allow header for any allowed set containing GET, mirroring
// computeAllowedMethods's contract.
func TestStaticGuards_HEADIsSynthesizedFromGET(t *testing.T) {
	cases := []struct {
		name    string
		allowed []string
		want    string
	}{
		{"GET only", []string{http.MethodGet}, "GET, HEAD"},
		{"POST only", []string{http.MethodPost}, "POST"},
		{"DELETE only", []string{http.MethodDelete}, "DELETE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Use(middleware.RequestID)
			register405Static(r, "/x", tc.allowed)
			// PATCH is always not in the allowed sets above, so always 405.
			rec := runReq(t, r, http.MethodPatch, "/x")
			require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
			assert.Equal(t, tc.want, rec.Header().Get("Allow"))
		})
	}
}
