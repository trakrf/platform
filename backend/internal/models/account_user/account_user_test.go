package account_user

import (
	"testing"
)

func TestAccountUserStruct(t *testing.T) {
	au := AccountUser{
		AccountID: 1,
		UserID:    2,
		Role:      "admin",
		Status:    "active",
	}

	if au.AccountID != 1 {
		t.Errorf("expected AccountID 1, got %d", au.AccountID)
	}
	if au.Role != "admin" {
		t.Errorf("expected role 'admin', got %s", au.Role)
	}
}
