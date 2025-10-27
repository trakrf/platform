package auth

import "github.com/golang-jwt/jwt/v5"

// JWTClaims represents the JWT payload structure
type JWTClaims struct {
	UserID       int    `json:"user_id"`
	Email        string `json:"email"`
	CurrentOrgID *int   `json:"current_org_id,omitempty"`
	jwt.RegisteredClaims
}
