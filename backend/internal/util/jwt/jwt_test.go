package jwt

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestGenerate(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-key")
	os.Setenv("JWT_EXPIRATION", "3600")

	userID := 1
	email := "test@example.com"
	orgID := 5

	token, err := Generate(userID, email, &orgID)

	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if token == "" {
		t.Error("token should not be empty")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("expected 3 JWT parts, got %d", len(parts))
	}
}

func TestValidate_Valid(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-key")
	os.Setenv("JWT_EXPIRATION", "3600")

	userID := 1
	email := "test@example.com"
	orgID := 5

	token, err := Generate(userID, email, &orgID)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	claims, err := Validate(token)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected UserID %d, got %d", userID, claims.UserID)
	}

	if claims.Email != email {
		t.Errorf("expected Email %s, got %s", email, claims.Email)
	}

	if claims.CurrentOrgID == nil || *claims.CurrentOrgID != orgID {
		t.Errorf("expected OrgID %d, got %v", orgID, claims.CurrentOrgID)
	}

	expectedExpiry := time.Now().Add(3600 * time.Second)
	expiryDiff := claims.ExpiresAt.Time.Sub(expectedExpiry)
	if expiryDiff > 5*time.Second || expiryDiff < -5*time.Second {
		t.Errorf("expiration time off by %v", expiryDiff)
	}
}

func TestValidate_Invalid(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-key")

	_, err := Validate("invalid.token.string")
	if err == nil {
		t.Error("Validate should fail for invalid token")
	}
}

func TestValidate_WrongSecret(t *testing.T) {
	os.Setenv("JWT_SECRET", "secret1")
	token, _ := Generate(1, "test@example.com", nil)

	os.Setenv("JWT_SECRET", "secret2")
	_, err := Validate(token)
	if err == nil {
		t.Error("Validate should fail when secret changes")
	}
}

func TestGetSecret_Default(t *testing.T) {
	os.Unsetenv("JWT_SECRET")

	secret := getSecret()
	if secret != "dev-secret-change-in-production" {
		t.Errorf("expected default secret, got %s", secret)
	}
}

func TestGetExpiration_Default(t *testing.T) {
	os.Unsetenv("JWT_EXPIRATION")

	exp := getExpiration()
	if exp != 3600 {
		t.Errorf("expected 3600 seconds default, got %d", exp)
	}
}

func TestGetExpiration_Custom(t *testing.T) {
	os.Setenv("JWT_EXPIRATION", "7200")

	exp := getExpiration()
	if exp != 7200 {
		t.Errorf("expected 7200 seconds, got %d", exp)
	}
}

func TestGetExpiration_Invalid(t *testing.T) {
	os.Setenv("JWT_EXPIRATION", "invalid")

	exp := getExpiration()
	if exp != 3600 {
		t.Errorf("expected fallback to 3600, got %d", exp)
	}
}
