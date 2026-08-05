package user

import (
	"testing"
)

func TestUserStruct(t *testing.T) {
	user := User{
		ID:    1,
		Email: "test@example.com",
		Name:  "Test User",
	}

	if user.ID != 1 {
		t.Errorf("expected ID 1, got %d", user.ID)
	}
	if user.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %s", user.Email)
	}
}
