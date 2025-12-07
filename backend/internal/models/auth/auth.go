package auth

import "github.com/trakrf/platform/backend/internal/models/user"

// SignupRequest for POST /api/v1/auth/signup
type SignupRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	// OrgName removed - auto-generated from email
}

// LoginRequest for POST /api/v1/auth/login
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// AuthResponse contains JWT token and user data
type AuthResponse struct {
	Token string    `json:"token"`
	User  user.User `json:"user"`
}

// ForgotPasswordRequest for POST /api/v1/auth/forgot-password
type ForgotPasswordRequest struct {
	Email     string `json:"email" validate:"required,email"`
	ResetURL  string `json:"reset_url" validate:"required,url"`
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
