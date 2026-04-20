package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/middleware"
	apierrors "github.com/trakrf/platform/backend/internal/models/errors"
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
	require.Equal(t, "Authentication required", resp.Error.Title)
	require.Equal(t, string(apierrors.ErrUnauthorized), resp.Error.Type)
	require.Equal(t, "Authorization header is required", resp.Error.Detail)
}
