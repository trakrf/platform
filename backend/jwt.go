package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims represents the JWT payload structure
type JWTClaims struct {
	UserID           int    `json:"user_id"`
	Email            string `json:"email"`
	CurrentAccountID *int   `json:"current_account_id,omitempty"`
	jwt.RegisteredClaims
}

// GenerateJWT creates a signed JWT token for authenticated user
func GenerateJWT(userID int, email string, accountID *int) (string, error) {
	expiration := getJWTExpiration()
	expirationTime := time.Now().Add(time.Duration(expiration) * time.Second)

	claims := &JWTClaims{
		UserID:           userID,
		Email:            email,
		CurrentAccountID: accountID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString([]byte(getJWTSecret()))
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return tokenString, nil
}

// ValidateJWT parses and validates a JWT token
func ValidateJWT(tokenString string) (*JWTClaims, error) {
	claims := &JWTClaims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(getJWTSecret()), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid JWT token")
	}

	return claims, nil
}

// getJWTSecret retrieves JWT signing secret from environment
func getJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		// Development fallback - MUST be overridden in production
		secret = "dev-secret-change-in-production"
	}
	return secret
}

// getJWTExpiration retrieves JWT expiration duration from environment (in seconds)
func getJWTExpiration() int {
	exp := os.Getenv("JWT_EXPIRATION")
	if exp == "" {
		return 3600 // 1 hour default
	}

	seconds, err := strconv.Atoi(exp)
	if err != nil || seconds <= 0 {
		return 3600 // fallback on invalid value
	}

	return seconds
}
