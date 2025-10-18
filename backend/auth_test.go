package main

import (
	"testing"
)

// TestSlugifyAccountName tests slug generation
func TestSlugifyAccountName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "My Company", "my-company"},
		{"uppercase", "ACME CORP", "acme-corp"},
		{"special chars", "Test! Company@", "test-company"},
		{"multiple spaces", "Test  Multiple   Spaces", "test-multiple-spaces"},
		{"leading/trailing", "-Leading Trailing-", "leading-trailing"},
		{"numbers", "Company123", "company123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slugifyAccountName(tt.input)
			if result != tt.expected {
				t.Errorf("slugifyAccountName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestSignup_ValidationErrors tests signup validation
func TestSignup_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		req     SignupRequest
		wantErr bool
	}{
		{"valid", SignupRequest{Email: "test@example.com", Password: "password123", AccountName: "Company"}, false},
		{"missing email", SignupRequest{Password: "password123", AccountName: "Company"}, true},
		{"invalid email", SignupRequest{Email: "notanemail", Password: "password123", AccountName: "Company"}, true},
		{"short password", SignupRequest{Email: "test@example.com", Password: "short", AccountName: "Company"}, true},
		{"missing account_name", SignupRequest{Email: "test@example.com", Password: "password123"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validate.Struct() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestLogin_ValidationErrors tests login validation
func TestLogin_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		req     LoginRequest
		wantErr bool
	}{
		{"valid", LoginRequest{Email: "test@example.com", Password: "password123"}, false},
		{"missing email", LoginRequest{Password: "password123"}, true},
		{"invalid email", LoginRequest{Email: "notanemail", Password: "password123"}, true},
		{"missing password", LoginRequest{Email: "test@example.com"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validate.Struct() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestPasswordHashing tests password utilities
func TestPasswordHashing(t *testing.T) {
	password := "mySecurePassword123"

	// Hash password
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	// Verify correct password
	err = ComparePassword(password, hash)
	if err != nil {
		t.Errorf("ComparePassword() should succeed for correct password, got error: %v", err)
	}

	// Verify incorrect password
	err = ComparePassword("wrongPassword", hash)
	if err == nil {
		t.Errorf("ComparePassword() should fail for incorrect password")
	}
}

// Note: Full integration tests with mocked database would go here
// For Phase 5B, we're keeping tests simple and focused on validation
// Phase 5C will add integration tests with the full auth flow
