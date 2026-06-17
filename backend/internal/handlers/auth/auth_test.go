package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authmodels "github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/models/organization"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
)

// stubAuthService implements authServicer for unit tests.
type stubAuthService struct {
	signupResult *authmodels.AuthResponse
	signupErr    error
	loginResult  *authmodels.AuthResponse
	loginErr     error
}

func (s *stubAuthService) Signup(_ context.Context, _ authmodels.SignupRequest, _, _ string, _ func(string) (string, error), _ func(int, string, *int) (string, error)) (*authmodels.AuthResponse, error) {
	return s.signupResult, s.signupErr
}

func (s *stubAuthService) Login(_ context.Context, _ authmodels.LoginRequest, _, _ string, _ func(string, string) error, _ func(int, string, *int) (string, error)) (*authmodels.AuthResponse, error) {
	return s.loginResult, s.loginErr
}

func (s *stubAuthService) Refresh(_ context.Context, _, _, _ string, _ func(int, string, *int) (string, error)) (*authmodels.RefreshResponse, error) {
	return nil, nil
}

func (s *stubAuthService) Logout(_ context.Context, _ string) error {
	return nil
}

func (s *stubAuthService) ForgotPassword(_ context.Context, _, _ string) error {
	return nil
}

func (s *stubAuthService) ResetPassword(_ context.Context, _, _ string, _ func(string) (string, error)) error {
	return nil
}

func (s *stubAuthService) AcceptInvitation(_ context.Context, _ string, _ int) (*organization.AcceptInvitationResponse, error) {
	return nil, nil
}

func (s *stubAuthService) GetInvitationInfo(_ context.Context, _ string) (*authmodels.InvitationInfoResponse, error) {
	return nil, nil
}

func (s *stubAuthService) MintAPITokenPair(_ context.Context, _ string, _ []string, _ int, _ int64, _, _ string) (string, string, int, error) {
	return "", "", 0, nil
}

func (s *stubAuthService) RefreshAPIToken(_ context.Context, _, _, _ string) (*authservice.APITokenResponse, error) {
	return nil, nil
}

// newTestHandler returns a Handler wired with the given stub service.
func newTestHandler(svc authServicer) *Handler {
	return &Handler{service: svc}
}

// errorBody is a minimal helper to parse the error envelope.
type errorBody struct {
	Error struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Detail string `json:"detail"`
		Fields []struct {
			Field   string `json:"field"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"fields"`
	} `json:"error"`
}

// TestSignup_MalformedBody_StableDetail verifies that a non-JSON body
// produces a stable 400 detail that does not leak encoding/json internals.
func TestSignup_MalformedBody_StableDetail(t *testing.T) {
	handler := newTestHandler(&stubAuthService{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Signup(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body errorBody
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	assert.Equal(t, "Request body is not valid JSON", body.Error.Detail,
		"detail must be the stable string, not a raw Go error")

	// Must NOT leak encoding/json parser internals.
	assert.NotContains(t, body.Error.Detail, "invalid character",
		"must not leak encoding/json internals")
	assert.NotContains(t, body.Error.Detail, "literal null",
		"must not leak encoding/json internals")
}

// TestSignup_EnvBlocked_Returns403 verifies that the service's ErrSignupNotAllowed
// (self-service signup blocked on a non-prod site, TRA-970) maps to a 403 with the
// stable go-to-production detail. The request body is fully valid so it passes
// validation and reaches the service.
func TestSignup_EnvBlocked_Returns403(t *testing.T) {
	handler := newTestHandler(&stubAuthService{signupErr: authservice.ErrSignupNotAllowed})

	body := `{"email":"new@example.com","password":"password123","org_name":"Acme","name":"Jane Doe","phone":"555-1234","website":"acme.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Signup(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp errorBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp.Error.Detail, "app.trakrf.id",
		"403 detail must point the visitor to the production site")
}

// TestSignup_BadBody_FieldsEnvelope verifies that validator errors produce
// the fields[] envelope with snake_case field names and mapped codes.
func TestSignup_BadBody_FieldsEnvelope(t *testing.T) {
	handler := newTestHandler(&stubAuthService{})

	body := `{"email":"not-an-email","password":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Signup(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp errorBody
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "validation_error", resp.Error.Type)

	// Build a map of field → code for easy assertions.
	fieldCodes := make(map[string]string)
	for _, f := range resp.Error.Fields {
		fieldCodes[f.Field] = f.Code
	}

	// "email" tag name (JSON tag, not Go struct name "Email")
	assert.Equal(t, "invalid_value", fieldCodes["email"],
		"email field must appear with snake_case JSON tag name and code=invalid_value")

	// "password" min=8 → too_short
	assert.Equal(t, "too_short", fieldCodes["password"],
		"password field must appear with code=too_short")

	// Field names must be snake_case (JSON tags), not Go struct names.
	for _, f := range resp.Error.Fields {
		assert.NotContains(t, f.Field, "Email",
			"field name must be JSON tag, not Go struct name")
		assert.NotContains(t, f.Field, "Password",
			"field name must be JSON tag, not Go struct name")
	}
}

// TestSignup_MissingContactFields_Rejected verifies that self-service signup
// (no invitation_token) requires name, phone, and website (TRA-971). A body with
// email/password/org_name but no contact fields must 400 with all three flagged.
func TestSignup_MissingContactFields_Rejected(t *testing.T) {
	handler := newTestHandler(&stubAuthService{})

	body := `{"email":"new@example.com","password":"password123","org_name":"Acme"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/signup", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Signup(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp errorBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	flagged := make(map[string]bool)
	for _, f := range resp.Error.Fields {
		flagged[f.Field] = true
	}
	assert.True(t, flagged["name"], "name must be required for self-service signup")
	assert.True(t, flagged["phone"], "phone must be required for self-service signup")
	assert.True(t, flagged["website"], "website must be required for self-service signup")
}

// TestLogin_WrongPassword_Respond401 verifies that a wrong-password service
// error is normalized via Respond401 with the correct header and body shape.
func TestLogin_WrongPassword_Respond401(t *testing.T) {
	stub := &stubAuthService{
		loginErr: errors.New("invalid email or password"),
	}
	handler := newTestHandler(stub)

	body := `{"email":"user@example.com","password":"wrongpass"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Login(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)

	// WWW-Authenticate header must be set.
	assert.Equal(t, `Bearer realm="trakrf-api"`, w.Header().Get("WWW-Authenticate"))

	var resp errorBody
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "Unauthorized", resp.Error.Title)
	assert.Equal(t, "Invalid email or password", resp.Error.Detail)
}
