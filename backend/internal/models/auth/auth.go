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
