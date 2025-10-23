package account

import (
	"testing"
)

func TestAccountStruct(t *testing.T) {
	account := Account{
		ID:     1,
		Name:   "Test Account",
		Domain: "test.example.com",
		Status: "active",
	}

	if account.ID != 1 {
		t.Errorf("expected ID 1, got %d", account.ID)
	}
	if account.Name != "Test Account" {
		t.Errorf("expected name 'Test Account', got %s", account.Name)
	}
}

func TestCreateAccountRequest(t *testing.T) {
	req := CreateAccountRequest{
		Name:         "Test Account",
		Domain:       "test.example.com",
		BillingEmail: "billing@test.com",
	}

	if req.Name == "" {
		t.Error("name should not be empty")
	}
}
