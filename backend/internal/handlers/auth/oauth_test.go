package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

// postTokenForm posts an application/x-www-form-urlencoded body to the token
// endpoint — the media type stock OAuth2 client libraries default to (RFC 6749
// §3.2/§4.4). Mirrors postToken but with a form body instead of JSON.
func postTokenForm(t *testing.T, h *Handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	h.RegisterRoutes(r, func(next http.Handler) http.Handler { return next })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestToken_AcceptsFormUrlencoded_ClientCredentials proves the handler parses a
// form-urlencoded body: grant_type is read from the form (routing to the
// client_credentials branch) and the missing client_id/client_secret yields 401
// — not the decode error a JSON-only handler returns for a form body.
func TestToken_AcceptsFormUrlencoded_ClientCredentials(t *testing.T) {
	h := newTestHandler(&stubAuthService{})
	w := postTokenForm(t, h, url.Values{"grant_type": {"client_credentials"}})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func postToken(t *testing.T, h *Handler, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	h.RegisterRoutes(r, func(next http.Handler) http.Handler { return next })

	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/oauth/token", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestToken_RejectsUnknownGrantType(t *testing.T) {
	h := newTestHandler(&stubAuthService{})
	w := postToken(t, h, map[string]string{"grant_type": "password"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestToken_ClientCredentialsRequiresClientFields(t *testing.T) {
	h := newTestHandler(&stubAuthService{})
	w := postToken(t, h, map[string]string{"grant_type": "client_credentials"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestToken_RefreshRequiresRefreshToken(t *testing.T) {
	h := newTestHandler(&stubAuthService{})
	w := postToken(t, h, map[string]string{"grant_type": "refresh_token"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestToken_MissingGrantTypeIs400(t *testing.T) {
	h := newTestHandler(&stubAuthService{})
	w := postToken(t, h, map[string]string{})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
