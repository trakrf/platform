package auth

import (
	"testing"
)

func TestSignupRequest(t *testing.T) {
	req := SignupRequest{
		Email:       "test@example.com",
		Password:    "password123",
		AccountName: "Test Account",
	}

	if req.Email == "" {
		t.Error("email should not be empty")
	}
	if len(req.Password) < 8 {
		t.Error("password should be at least 8 characters")
	}
}

func TestLoginRequest(t *testing.T) {
	req := LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
	}

	if req.Email == "" {
		t.Error("email should not be empty")
	}
}
