package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/middleware"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

// newChangePasswordRequest builds an authenticated PUT /api/v1/auth/password
// request with the given body, with JWT claims for user 42 injected the way
// the jwt middleware would.
func newChangePasswordRequest(t *testing.T, body string, withClaims bool) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/auth/password", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if withClaims {
		claims := &jwt.Claims{UserID: 42}
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserClaimsKey, claims))
	}
	return req
}

// TestChangePassword_NoClaims_401 verifies the handler refuses a request that
// reached it without JWT claims in context.
func TestChangePassword_NoClaims_401(t *testing.T) {
	handler := newTestHandler(&stubAuthService{})

	req := newChangePasswordRequest(t, `{"current_password":"oldpass123","new_password":"newpass123"}`, false)
	w := httptest.NewRecorder()

	handler.ChangePassword(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestChangePassword_ShortNewPassword_FieldsEnvelope verifies new_password
// carries the same min=8 rule as signup/reset and surfaces in the fields[]
// envelope under its JSON tag name.
func TestChangePassword_ShortNewPassword_FieldsEnvelope(t *testing.T) {
	handler := newTestHandler(&stubAuthService{})

	req := newChangePasswordRequest(t, `{"current_password":"oldpass123","new_password":"short"}`, true)
	w := httptest.NewRecorder()

	handler.ChangePassword(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp errorBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "validation_error", resp.Error.Type)

	fieldCodes := make(map[string]string)
	for _, f := range resp.Error.Fields {
		fieldCodes[f.Field] = f.Code
	}
	assert.Equal(t, "too_short", fieldCodes["new_password"],
		"new_password must appear with snake_case JSON tag name and code=too_short")
}

// TestChangePassword_WrongCurrent_400StableDetail verifies the service's
// ErrInvalidCurrentPassword maps to a 400 with a stable detail string, not a
// 401 (the caller IS authenticated — a 401 would trip token-refresh/logout
// handling in clients).
func TestChangePassword_WrongCurrent_400StableDetail(t *testing.T) {
	handler := newTestHandler(&stubAuthService{changePasswordErr: authservice.ErrInvalidCurrentPassword})

	req := newChangePasswordRequest(t, `{"current_password":"wrongpass1","new_password":"newpass123"}`, true)
	w := httptest.NewRecorder()

	handler.ChangePassword(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp errorBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Current password is incorrect", resp.Error.Detail)
}

// TestChangePassword_Success_200Message verifies the happy path returns the
// standard message envelope.
func TestChangePassword_Success_200Message(t *testing.T) {
	handler := newTestHandler(&stubAuthService{})

	req := newChangePasswordRequest(t, `{"current_password":"oldpass123","new_password":"newpass123"}`, true)
	w := httptest.NewRecorder()

	handler.ChangePassword(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Password updated successfully", resp.Message)
}
