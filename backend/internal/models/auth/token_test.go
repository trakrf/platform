package auth

import (
	"testing"
)

func TestJWTClaims(t *testing.T) {
	orgID := 1
	claims := JWTClaims{
		UserID:       123,
		Email:        "test@example.com",
		CurrentOrgID: &orgID,
	}

	if claims.UserID != 123 {
		t.Errorf("expected UserID 123, got %d", claims.UserID)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %s", claims.Email)
	}
	if *claims.CurrentOrgID != 1 {
		t.Errorf("expected CurrentOrgID 1, got %d", *claims.CurrentOrgID)
	}
}
