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

// firstFieldCode pulls error.fields[0].code out of a validation envelope.
func firstFieldCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Fields []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"fields"`
		} `json:"error"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	if len(body.Error.Fields) == 0 {
		t.Fatalf("expected at least one field error, got body: %s", w.Body.String())
	}
	assert.Equal(t, "grant_type", body.Error.Fields[0].Field)
	return body.Error.Fields[0].Code
}

// TRA-877 item 4: an ABSENT grant_type is a presence violation → code "required",
// matching the rest of the surface (POST /assets with no name → required) and the
// errors-doc taxonomy. Three of three bb-2.3 sessions reproduced the old too_short.
func TestToken_AbsentGrantType_ReturnsRequired(t *testing.T) {
	h := newTestHandler(&stubAuthService{})
	w := postToken(t, h, map[string]string{})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "required", firstFieldCode(t, w),
		"absent grant_type must report code=required, not too_short")
}

// An EMPTY grant_type ("") is present-but-invalid-length → stays too_short,
// mirroring empty name on POST /assets. (The presence overlay must not over-promote.)
func TestToken_EmptyGrantType_StaysTooShort(t *testing.T) {
	h := newTestHandler(&stubAuthService{})
	w := postToken(t, h, map[string]string{"grant_type": ""})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "too_short", firstFieldCode(t, w),
		"empty grant_type was provided (just too short) — not a presence violation")
}

// The form-urlencoded path must honor the same taxonomy: absent grant_type → required.
func TestToken_AbsentGrantTypeForm_ReturnsRequired(t *testing.T) {
	h := newTestHandler(&stubAuthService{})
	w := postTokenForm(t, h, url.Values{})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "required", firstFieldCode(t, w),
		"absent grant_type on a form body must also report code=required")
}
