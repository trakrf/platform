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

// InvitationInfoResponse for GET /api/v1/auth/invitation-info
type InvitationInfoResponse struct {
	OrgName       string  `json:"org_name"`
	OrgIdentifier string  `json:"org_identifier"`
	Role          string  `json:"role"`
	Email         string  `json:"email"`
	UserExists    bool    `json:"user_exists"`
	InviterName   *string `json:"inviter_name,omitempty"`
}
