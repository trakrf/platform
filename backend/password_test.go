package main

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "testpassword123"
	hash, err := HashPassword(password)

	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == "" {
		t.Error("hash should not be empty")
	}

	if hash == password {
		t.Error("hash should not equal plain password")
	}

	// bcrypt hash is always 60 characters
	if len(hash) != 60 {
		t.Errorf("expected hash length 60, got %d", len(hash))
	}

	// Should start with bcrypt identifier ($2a$ or $2b$)
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") {
		t.Errorf("hash should start with bcrypt identifier, got: %s", hash[:4])
	}
}

func TestComparePassword_Valid(t *testing.T) {
	password := "testpassword123"
	hash, _ := HashPassword(password)

	err := ComparePassword(password, hash)
	if err != nil {
		t.Errorf("ComparePassword should succeed for valid password: %v", err)
	}
}

func TestComparePassword_Invalid(t *testing.T) {
	password := "testpassword123"
	hash, _ := HashPassword(password)

	err := ComparePassword("wrongpassword", hash)
	if err == nil {
		t.Error("ComparePassword should fail for invalid password")
	}
}
