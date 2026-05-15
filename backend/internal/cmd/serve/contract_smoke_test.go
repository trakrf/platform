package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
	"github.com/trakrf/platform/backend/internal/ratelimit"
	"github.com/trakrf/platform/backend/internal/util/httputil"
)

var ulidRE = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)

// TestContract_RequestIDIsULIDAndPropagates verifies that when no inbound
// X-Request-ID is supplied, the RequestID middleware generates a ULID that
// appears both in the X-Request-ID response header and (when downstream
// emits an error envelope) the request_id body field.
func TestContract_RequestIDIsULIDAndPropagates(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Auth) // Auth fires a 401 with no Authorization header.
	r.Get("/protected", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)

	hdr := rec.Header().Get("X-Request-ID")
	require.True(t, ulidRE.MatchString(hdr),
		"X-Request-ID = %q, want 26-char Crockford base32 ULID", hdr)

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, hdr, resp.Error.RequestID,
		"request_id in body does not match X-Request-ID header")
}

// TestContract_MethodNotAllowed_EmitsEnvelope asserts that an unknown method
// against an existing route returns the documented envelope (TRA-541 §1.10).
// Before the fix, chi's default 405 handler returned an empty body.
func TestContract_MethodNotAllowed_EmitsEnvelope(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		allowed := computeAllowedMethods(r, req.URL.Path)
		httputil.Respond405(w, req, allowed, middleware.GetRequestID(req.Context()))
	})
	r.Get("/api/v1/assets", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Post("/api/v1/assets", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/assets", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.NotEmpty(t, rec.Body.String(), "405 must carry an envelope, not an empty body")
	require.Equal(t, "GET, HEAD, POST", rec.Header().Get("Allow"),
		"Allow header must enumerate the methods the route accepts (HEAD synthesized from GET)")

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "method_not_allowed", resp.Error.Type)
	require.Equal(t, 405, resp.Error.Status)
	require.Equal(t, "Method not allowed", resp.Error.Title)
	require.Equal(t, "Allowed methods: GET, HEAD, POST", resp.Error.Detail)
}

// TestContract_MissingAuthHeader_WWWAuthenticate verifies that a request to an
// Auth-protected route with no Authorization header exits with the documented
// 401 envelope AND WWW-Authenticate: Bearer realm="trakrf-api", confirming
// the session Auth middleware is actually routing through Respond401.
func TestContract_MissingAuthHeader_WWWAuthenticate(t *testing.T) {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Auth)
	r.Get("/protected", func(w http.ResponseWriter, req *http.Request) {
		t.Fatal("handler should not be reached")
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Equal(t, `Bearer realm="trakrf-api"`, rec.Header().Get("WWW-Authenticate"))

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "Unauthorized", resp.Error.Title)
	require.Equal(t, string(apierrors.ErrUnauthorized), resp.Error.Type)
	require.Equal(t, middleware.Detail401MissingAuthHeader, resp.Error.Detail)
}

// TestContract_BB12_401Reproductions covers the four 401 variants named in
// BB12 §1.2 (TRA-538). Each must emit title="Unauthorized" with
// the variable explanation in detail. Title must NOT contain "Bearer" or
// the offending value — the contract violation the audit found.
func TestContract_BB12_401Reproductions(t *testing.T) {
	// Stand-in for EitherAuth that mirrors its four 401 paths but runs
	// without a DB. Real EitherAuth is integration-tagged in
	// either_auth_test.go; this test guards the contract end-to-end.
	auth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := middleware.GetRequestID(r.Context())
			h := r.Header.Get("Authorization")
			if h == "" {
				detail := middleware.Detail401MissingAuthHeader
				if r.Header.Get("X-API-Key") != "" {
					detail = middleware.Detail401UseAuthBearerHint
				}
				httputil.Respond401(w, r, detail, reqID)
				return
			}
			parts := strings.SplitN(h, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				httputil.Respond401(w, r, middleware.Detail401InvalidAuthFormat, reqID)
				return
			}
			httputil.Respond401(w, r, middleware.Detail401InvalidOrExpiredToken, reqID)
		})
	}

	mkRouter := func() *chi.Mux {
		mux := chi.NewRouter()
		mux.Use(middleware.RequestID)
		mux.Use(auth)
		mux.Get("/api/v1/assets", func(w http.ResponseWriter, req *http.Request) {
			t.Fatal("auth should reject before handler")
		})
		return mux
	}

	cases := []struct {
		name           string
		setup          func(*http.Request)
		wantDetailHas  string
		titleMustNotBe []string
	}{
		{
			name:           "missing header",
			setup:          func(r *http.Request) {},
			wantDetailHas:  middleware.Detail401MissingAuthHeader,
			titleMustNotBe: []string{"Bearer", middleware.Detail401MissingAuthHeader},
		},
		{
			name:           "wrong scheme",
			setup:          func(r *http.Request) { r.Header.Set("Authorization", "Basic abc") },
			wantDetailHas:  middleware.Detail401InvalidAuthFormat,
			titleMustNotBe: []string{"Basic", middleware.Detail401InvalidAuthFormat},
		},
		{
			name:           "garbage token",
			setup:          func(r *http.Request) { r.Header.Set("Authorization", "Bearer not-a-jwt") },
			wantDetailHas:  middleware.Detail401InvalidOrExpiredToken,
			titleMustNotBe: []string{"Bearer not-a-jwt", middleware.Detail401InvalidOrExpiredToken},
		},
		{
			name:           "missing header with X-API-Key",
			setup:          func(r *http.Request) { r.Header.Set("X-API-Key", "some-token") },
			wantDetailHas:  middleware.Detail401UseAuthBearerHint,
			titleMustNotBe: []string{"Bearer <token>", middleware.Detail401UseAuthBearerHint},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
			tc.setup(req)
			rec := httptest.NewRecorder()
			mkRouter().ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code)

			var resp apierrors.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

			require.Equal(t, "Unauthorized", resp.Error.Title,
				"title must be the fixed string per TRA-538 contract")
			require.Equal(t, "unauthorized", resp.Error.Type)
			require.Contains(t, resp.Error.Detail, tc.wantDetailHas,
				"variable explanation must live in detail")
			for _, forbidden := range tc.titleMustNotBe {
				require.NotContains(t, resp.Error.Title, forbidden,
					"title must not contain the variable substring %q", forbidden)
			}
		})
	}
}

// TestContract_TRA724_401DetailsAreEndpointAgnostic locks in the TRA-724
// harmonization: the missing-header and malformed-JWT 401 cases must emit
// the same canonical detail string regardless of which middleware chain the
// route is fronted by. The four routes named in the ticket cover both
// chains in production — `/orgs/me` is APIKeyAuth-only, the rest are
// EitherAuth-fronted. The bug that motivated this ticket was the two chains
// diverging on the same condition.
//
// The auth chain runs to its terminal 401 without a DB for these inputs
// (no Authorization header → APIKeyAuth/EitherAuth return early;
// "Bearer not-a-jwt" → ValidateAPIKey/ClassifyToken fail before any
// storage call), so nil store is safe.
func TestContract_TRA724_401DetailsAreEndpointAgnostic(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra724-test-secret")

	build := func() *chi.Mux {
		mux := chi.NewRouter()
		mux.Use(middleware.RequestID)
		mux.With(middleware.APIKeyAuth(nil)).Get("/api/v1/orgs/me",
			func(w http.ResponseWriter, r *http.Request) { t.Fatal("auth should reject before handler") })
		mux.Group(func(r chi.Router) {
			r.Use(middleware.EitherAuth(nil))
			r.Get("/api/v1/assets", func(w http.ResponseWriter, r *http.Request) { t.Fatal("auth should reject before handler") })
			r.Get("/api/v1/locations", func(w http.ResponseWriter, r *http.Request) { t.Fatal("auth should reject before handler") })
			r.Get("/api/v1/reports/asset-locations", func(w http.ResponseWriter, r *http.Request) { t.Fatal("auth should reject before handler") })
		})
		return mux
	}

	endpoints := []string{
		"/api/v1/orgs/me",
		"/api/v1/assets",
		"/api/v1/locations",
		"/api/v1/reports/asset-locations",
	}

	cases := []struct {
		name       string
		setReq     func(*http.Request)
		wantDetail string
	}{
		{
			name:       "missing Authorization header",
			setReq:     func(r *http.Request) {},
			wantDetail: middleware.Detail401MissingAuthHeader,
		},
		{
			name:       "malformed JWT in Bearer token",
			setReq:     func(r *http.Request) { r.Header.Set("Authorization", "Bearer not-a-jwt") },
			wantDetail: middleware.Detail401InvalidOrExpiredToken,
		},
	}

	mux := build()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range endpoints {
				t.Run(path, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodGet, path, nil)
					tc.setReq(req)
					rec := httptest.NewRecorder()
					mux.ServeHTTP(rec, req)

					require.Equal(t, http.StatusUnauthorized, rec.Code)
					require.Equal(t, `Bearer realm="trakrf-api"`, rec.Header().Get("WWW-Authenticate"))

					var resp apierrors.ErrorResponse
					require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
					require.Equal(t, "Unauthorized", resp.Error.Title)
					require.Equal(t, string(apierrors.ErrUnauthorized), resp.Error.Type)
					require.Equal(t, tc.wantDetail, resp.Error.Detail,
						"401 detail must be identical across every endpoint for the same auth-failure condition (TRA-724)")
				})
			}
		})
	}
}

// TestContract_NotFound_FixedTitleAndDetail covers TRA-538: 404 responses
// must emit title="Not found" with the resource-specific message in detail.
func TestContract_NotFound_FixedTitleAndDetail(t *testing.T) {
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Get("/api/v1/assets/{asset_id}", func(w http.ResponseWriter, req *http.Request) {
		httputil.Respond404(w, req, "Asset not found",
			middleware.GetRequestID(req.Context()))
	})
	mux.Get("/api/*", func(w http.ResponseWriter, req *http.Request) {
		httputil.Respond404(w, req, "Unknown API route: "+req.URL.Path,
			middleware.GetRequestID(req.Context()))
	})

	cases := []struct {
		name          string
		path          string
		wantDetailHas string
		titleMustNot  string
	}{
		{"handler 404", "/api/v1/assets/bogus", "Asset not found", "Asset not found"},
		{"catchall 404", "/api/v1/nonexistent", "Unknown API route", "Unknown API route"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			require.Equal(t, http.StatusNotFound, rec.Code)

			var resp apierrors.ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Equal(t, "Not found", resp.Error.Title,
				"title must be the fixed string per TRA-538 contract")
			require.Equal(t, "not_found", resp.Error.Type)
			require.Contains(t, resp.Error.Detail, tc.wantDetailHas)
			require.NotContains(t, resp.Error.Title, tc.titleMustNot,
				"title must not contain the variable string")
		})
	}
}

// TestContract_UnsupportedMediaType_EnvelopeAndType covers TRA-541 §1.11:
// 415 must emit type=unsupported_media_type and a detail that does not
// name multipart (since the public OpenAPI spec contains no multipart
// endpoints).
func TestContract_UnsupportedMediaType_EnvelopeAndType(t *testing.T) {
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Use(middleware.ContentType)
	mux.Post("/api/v1/assets", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", nil)
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "unsupported_media_type", resp.Error.Type)
	require.Equal(t, "Unsupported media type", resp.Error.Title)
	require.Equal(t, 415, resp.Error.Status)
	require.NotContains(t, resp.Error.Detail, "multipart",
		"public 415 detail must not name multipart per TRA-541 POLS resolution")
}

// TestContract_UnsupportedMediaType_CarriesRateLimitHeaders covers TRA-703
// / BB32 C1: 415 responses on the public API surface must include the three
// X-RateLimit-* headers the Rate Limits docs commit to. The chain shape
// mirrors setupRouter: APIv1DefaultRateLimitHeaders runs as a global before
// ContentType, so the 415 rejection carries headers from the upstream stamp.
func TestContract_UnsupportedMediaType_CarriesRateLimitHeaders(t *testing.T) {
	clock := ratelimit.NewFakeClock(time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC))
	lim := ratelimit.NewLimiter(ratelimit.Config{
		RatePerMinute: 60,
		Burst:         120,
		IdleTTL:       time.Hour,
		SweepInterval: 24 * time.Hour,
		Clock:         clock,
	})
	defer lim.Close()

	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Use(middleware.APIv1DefaultRateLimitHeaders(lim))
	mux.Use(middleware.ContentType)
	mux.Post("/api/v1/assets", func(w http.ResponseWriter, req *http.Request) {
		t.Fatal("handler must not run on 415 rejection")
	})

	// Tests every CT shape ContentType rejects on POST: empty, merge-patch+json,
	// multipart on a non-bulk path, and an arbitrary foreign type.
	cases := []struct {
		name string
		ct   string
	}{
		{"empty CT", ""},
		{"merge-patch+json on POST", "application/merge-patch+json"},
		{"multipart on public POST", "multipart/form-data; boundary=----X"},
		{"text/plain", "text/plain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/assets", nil)
			if tc.ct != "" {
				req.Header.Set("Content-Type", tc.ct)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
			require.NotEmpty(t, rec.Header().Get("X-RateLimit-Limit"),
				"X-RateLimit-Limit must be set on 415 (BB32 C1)")
			require.NotEmpty(t, rec.Header().Get("X-RateLimit-Remaining"),
				"X-RateLimit-Remaining must be set on 415 (BB32 C1)")
			require.NotEmpty(t, rec.Header().Get("X-RateLimit-Reset"),
				"X-RateLimit-Reset must be set on 415 (BB32 C1)")
		})
	}
}

// TestContract_UnsupportedMediaType_NonAPIPathHasNoRateLimitHeaders pins the
// path-gated behavior of APIv1DefaultRateLimitHeaders: non-/api/v1/* paths
// don't carry rate-limit headers, preserving the TRA-518 design choice to
// scope those headers to the public API surface.
func TestContract_UnsupportedMediaType_NonAPIPathHasNoRateLimitHeaders(t *testing.T) {
	clock := ratelimit.NewFakeClock(time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC))
	lim := ratelimit.NewLimiter(ratelimit.Config{
		RatePerMinute: 60,
		Burst:         120,
		IdleTTL:       time.Hour,
		SweepInterval: 24 * time.Hour,
		Clock:         clock,
	})
	defer lim.Close()

	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Use(middleware.APIv1DefaultRateLimitHeaders(lim))
	mux.Use(middleware.ContentType)
	mux.Post("/some-other-path", func(w http.ResponseWriter, req *http.Request) {
		t.Fatal("handler must not run on 415")
	})

	req := httptest.NewRequest(http.MethodPost, "/some-other-path", nil)
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	require.Empty(t, rec.Header().Get("X-RateLimit-Limit"),
		"non-/api/v1 paths must not carry rate-limit headers")
}

// TestContract_MissingOrgContext_EnvelopeAndType covers TRA-537 follow-up:
// 422 with type=missing_org_context for the "auth ok but org missing"
// state. The test wires RespondMissingOrgContext through chi to confirm
// the helper produces the documented envelope shape end-to-end.
func TestContract_MissingOrgContext_EnvelopeAndType(t *testing.T) {
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Get("/api/v1/assets", func(w http.ResponseWriter, req *http.Request) {
		httputil.RespondMissingOrgContext(w, req, middleware.GetRequestID(req.Context()))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/assets", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "missing_org_context", resp.Error.Type)
	require.Equal(t, "Missing org context", resp.Error.Title)
	require.Equal(t, 422, resp.Error.Status)
	require.Contains(t, resp.Error.Detail, "active organization context")
	require.NotEmpty(t, resp.Error.RequestID, "request_id must propagate into envelope")
}
