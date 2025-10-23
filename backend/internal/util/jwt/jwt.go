package jwt

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID           int    `json:"user_id"`
	Email            string `json:"email"`
	CurrentAccountID *int   `json:"current_account_id,omitempty"`
	jwt.RegisteredClaims
}

// Generate creates a signed JWT token for an authenticated user.
func Generate(userID int, email string, accountID *int) (string, error) {
	expiration := getExpiration()
	expirationTime := time.Now().Add(time.Duration(expiration) * time.Second)

	claims := &Claims{
		UserID:           userID,
		Email:            email,
		CurrentAccountID: accountID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString([]byte(getSecret()))
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	return tokenString, nil
}

// Validate parses and validates a JWT token.
func Validate(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(getSecret()), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid JWT token")
	}

	return claims, nil
}

func getSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret-change-in-production"
	}
	return secret
}

func getExpiration() int {
	exp := os.Getenv("JWT_EXPIRATION")
	if exp == "" {
		return 3600
	}

	seconds, err := strconv.Atoi(exp)
	if err != nil || seconds <= 0 {
		return 3600
	}

	return seconds
}
