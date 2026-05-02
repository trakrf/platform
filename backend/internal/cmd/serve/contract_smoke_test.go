package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
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
		httputil.Respond405(w, req, middleware.GetRequestID(req.Context()))
	})
	r.Get("/api/v1/assets", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/assets", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.NotEmpty(t, rec.Body.String(), "405 must carry an envelope, not an empty body")

	var resp apierrors.ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "method_not_allowed", resp.Error.Type)
	require.Equal(t, 405, resp.Error.Status)
	require.Equal(t, "Method not allowed", resp.Error.Title)
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
	require.Equal(t, "Authorization header is required", resp.Error.Detail)
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
				detail := "Missing authorization header"
				if r.Header.Get("X-API-Key") != "" {
					detail = "Use Authorization: Bearer <token>"
				}
				httputil.Respond401(w, r, detail, reqID)
				return
			}
			parts := strings.SplitN(h, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				httputil.Respond401(w, r, "Invalid authorization header format", reqID)
				return
			}
			httputil.Respond401(w, r, "Invalid or expired token", reqID)
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
			wantDetailHas:  "Missing authorization header",
			titleMustNotBe: []string{"Bearer", "Missing authorization header"},
		},
		{
			name:           "wrong scheme",
			setup:          func(r *http.Request) { r.Header.Set("Authorization", "Basic abc") },
			wantDetailHas:  "Invalid authorization header format",
			titleMustNotBe: []string{"Basic", "Invalid authorization header format"},
		},
		{
			name:           "garbage token",
			setup:          func(r *http.Request) { r.Header.Set("Authorization", "Bearer not-a-jwt") },
			wantDetailHas:  "Invalid or expired token",
			titleMustNotBe: []string{"Bearer not-a-jwt", "Invalid or expired token"},
		},
		{
			name:           "missing header with X-API-Key",
			setup:          func(r *http.Request) { r.Header.Set("X-API-Key", "some-token") },
			wantDetailHas:  "Use Authorization: Bearer <token>",
			titleMustNotBe: []string{"Bearer <token>", "Use Authorization: Bearer <token>"},
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

// TestContract_NotFound_FixedTitleAndDetail covers TRA-538: 404 responses
// must emit title="Not found" with the resource-specific message in detail.
func TestContract_NotFound_FixedTitleAndDetail(t *testing.T) {
	mux := chi.NewRouter()
	mux.Use(middleware.RequestID)
	mux.Get("/api/v1/assets/{id}", func(w http.ResponseWriter, req *http.Request) {
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
