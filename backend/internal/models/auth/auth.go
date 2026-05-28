package auth

import "github.com/trakrf/platform/backend/internal/models/user"

// SignupRequest for POST /api/v1/auth/signup
// OrgName is required only when InvitationToken is not provided
type SignupRequest struct {
	Email           string  `json:"email" validate:"required,email"`
	Password        string  `json:"password" validate:"required,min=8"`
	OrgName         string  `json:"org_name" validate:"required_without=InvitationToken,omitempty,min=2,max=100"`
	InvitationToken *string `json:"invitation_token,omitempty"`
}

// LoginRequest for POST /api/v1/auth/login
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// AuthResponse contains an access JWT, a refresh token, the access TTL in
// seconds, and the user record. Returned from signup and login.
//
// Shape parallels OAuth2: access_token is the short-lived bearer (JWT);
// refresh_token is an opaque rotating secret to exchange via /auth/refresh
// when the access JWT expires.
type AuthResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresIn    int       `json:"expires_in"`
	User         user.User `json:"user"`
}

// RefreshRequest is the body for POST /api/v1/auth/refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// RefreshResponse is the body returned by POST /api/v1/auth/refresh.
// Same fields as AuthResponse minus the user record (the caller already has it).
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// LogoutRequest is the body for POST /api/v1/auth/logout. The access JWT
// authenticates the caller; the refresh_token is the rotating secret to
// revoke server-side.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// ForgotPasswordRequest for POST /api/v1/auth/forgot-password
type ForgotPasswordRequest struct {
	Email    string `json:"email" validate:"required,email"`
	ResetURL string `json:"reset_url" validate:"required,url"`
}

// ResetPasswordRequest for POST /api/v1/auth/reset-password
type ResetPasswordRequest struct {
	Token    string `json:"token" validate:"required"`
	Password string `json:"password" validate:"required,min=8"`
}

// MessageResponse for simple success/error messages
type MessageResponse struct {
	Message string `json:"message"`
}

// TokenRequest is the body for POST /api/v1/oauth/token. The OAuth2 grant is
// selected by grant_type; the remaining fields are conditionally required.
type TokenRequest struct {
	GrantType    string `json:"grant_type" validate:"required,oneof=client_credentials refresh_token" example:"client_credentials"`
	ClientID     string `json:"client_id,omitempty" example:"6f1c2a8e-7d3b-4e90-9a11-2c4d5e6f7a8b"`
	ClientSecret string `json:"client_secret,omitempty" example:"trakrf_9f8e7d6c5b4a39281706f5e4d3c2b1a0ffeeddccbbaa99887766554433221100"`
	RefreshToken string `json:"refresh_token,omitempty" example:"f3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`
}

// TokenResponse is the OAuth2 token grant result for the public API.
type TokenResponse struct {
	AccessToken  string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.short-lived-access-jwt"`
	RefreshToken string `json:"refresh_token" example:"f3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`
	TokenType    string `json:"token_type" example:"Bearer"`
	ExpiresIn    int    `json:"expires_in" example:"900"`
}

// InvitationInfoResponse for GET /api/v1/auth/invitation-info
type InvitationInfoResponse struct {
	OrgName       string  `json:"org_name"`
	OrgIdentifier string  `json:"org_identifier"`
	Role          string  `json:"role"`
	Email         string  `json:"email"`
	UserExists    bool    `json:"user_exists"`
	InviterName   *string `json:"inviter_name,omitempty"`
}
