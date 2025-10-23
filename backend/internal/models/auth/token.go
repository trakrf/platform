package auth

import "github.com/golang-jwt/jwt/v5"

// JWTClaims represents the JWT payload structure
type JWTClaims struct {
	UserID           int    `json:"user_id"`
	Email            string `json:"email"`
	CurrentAccountID *int   `json:"current_account_id,omitempty"`
	jwt.RegisteredClaims
}
