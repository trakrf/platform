package password

import (
	"strings"
	"testing"
)

func TestHash(t *testing.T) {
	password := "testpassword123"
	hash, err := Hash(password)

	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	if hash == "" {
		t.Error("hash should not be empty")
	}

	if hash == password {
		t.Error("hash should not equal plain password")
	}

	if len(hash) != 60 {
		t.Errorf("expected hash length 60, got %d", len(hash))
	}

	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") {
		t.Errorf("hash should start with bcrypt identifier, got: %s", hash[:4])
	}
}

func TestCompare_Valid(t *testing.T) {
	password := "testpassword123"
	hash, _ := Hash(password)

	err := Compare(password, hash)
	if err != nil {
		t.Errorf("Compare should succeed for valid password: %v", err)
	}
}

func TestCompare_Invalid(t *testing.T) {
	password := "testpassword123"
	hash, _ := Hash(password)

	err := Compare("wrongpassword", hash)
	if err == nil {
		t.Error("Compare should fail for invalid password")
	}
}
