package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestGenerateJWT(t *testing.T) {
	// Set test environment
	os.Setenv("JWT_SECRET", "test-secret-key")
	os.Setenv("JWT_EXPIRATION", "3600")

	userID := 1
	email := "test@example.com"
	accountID := 5

	token, err := GenerateJWT(userID, email, &accountID)

	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	if token == "" {
		t.Error("token should not be empty")
	}

	// JWT format: header.payload.signature (3 parts separated by dots)
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("expected 3 JWT parts, got %d", len(parts))
	}
}

func TestValidateJWT_Valid(t *testing.T) {
	// Set test environment
	os.Setenv("JWT_SECRET", "test-secret-key")
	os.Setenv("JWT_EXPIRATION", "3600")

	userID := 1
	email := "test@example.com"
	accountID := 5

	// Generate token
	token, err := GenerateJWT(userID, email, &accountID)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	// Validate token
	claims, err := ValidateJWT(token)
	if err != nil {
		t.Fatalf("ValidateJWT failed: %v", err)
	}

	// Verify claims
	if claims.UserID != userID {
		t.Errorf("expected UserID %d, got %d", userID, claims.UserID)
	}

	if claims.Email != email {
		t.Errorf("expected Email %s, got %s", email, claims.Email)
	}

	if claims.CurrentAccountID == nil || *claims.CurrentAccountID != accountID {
		t.Errorf("expected AccountID %d, got %v", accountID, claims.CurrentAccountID)
	}

	// Verify expiration is ~1 hour from now
	expectedExpiry := time.Now().Add(3600 * time.Second)
	expiryDiff := claims.ExpiresAt.Time.Sub(expectedExpiry)
	if expiryDiff > 5*time.Second || expiryDiff < -5*time.Second {
		t.Errorf("expiration time off by %v", expiryDiff)
	}
}

func TestValidateJWT_Invalid(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-key")

	// Try to validate malformed token
	_, err := ValidateJWT("invalid.token.string")
	if err == nil {
		t.Error("ValidateJWT should fail for invalid token")
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
	// Generate with one secret
	os.Setenv("JWT_SECRET", "secret1")
	token, _ := GenerateJWT(1, "test@example.com", nil)

	// Validate with different secret
	os.Setenv("JWT_SECRET", "secret2")
	_, err := ValidateJWT(token)
	if err == nil {
		t.Error("ValidateJWT should fail when secret changes")
	}
}

func TestGetJWTSecret_Default(t *testing.T) {
	os.Unsetenv("JWT_SECRET")

	secret := getJWTSecret()
	if secret != "dev-secret-change-in-production" {
		t.Errorf("expected default secret, got %s", secret)
	}
}

func TestGetJWTExpiration_Default(t *testing.T) {
	os.Unsetenv("JWT_EXPIRATION")

	exp := getJWTExpiration()
	if exp != 3600 {
		t.Errorf("expected 3600 seconds default, got %d", exp)
	}
}

func TestGetJWTExpiration_Custom(t *testing.T) {
	os.Setenv("JWT_EXPIRATION", "7200")

	exp := getJWTExpiration()
	if exp != 7200 {
		t.Errorf("expected 7200 seconds, got %d", exp)
	}
}

func TestGetJWTExpiration_Invalid(t *testing.T) {
	os.Setenv("JWT_EXPIRATION", "invalid")

	exp := getJWTExpiration()
	if exp != 3600 {
		t.Errorf("expected fallback to 3600, got %d", exp)
	}
}
