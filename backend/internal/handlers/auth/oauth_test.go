package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

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
