package auth

import (
	"testing"
)

func TestSignup(t *testing.T) {
	t.Skip("Requires test database and dependencies - implement in integration tests")
}

func TestLogin(t *testing.T) {
	t.Skip("Requires test database and dependencies - implement in integration tests")
}

func TestSlugifyOrgName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Organization names
		{"simple name", "My Company", "my-company"},
		{"special chars", "ACME Corp!", "acme-corp"},
		{"multiple spaces", "Test  Multiple   Spaces", "test-multiple-spaces"},
		{"numbers", "Company123", "company123"},
		{"hyphens already", "my-company", "my-company"},
		// Email addresses (primary use case)
		{"simple email", "mike@example.com", "mike-example-com"},
		{"email with dots", "alice.smith@company.io", "alice-smith-company-io"},
		{"email with plus", "bob+test@gmail.com", "bob-test-gmail-com"},
		{"email with hyphens", "john-doe@example.com", "john-doe-example-com"},
		{"uppercase email", "MIKE@EXAMPLE.COM", "mike-example-com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slugifyOrgName(tt.input)
			if result != tt.expected {
				t.Errorf("slugifyOrgName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TODO: Add integration test for Signup() that:
// 1. Creates user + personal org in transaction
// 2. Verifies is_personal = true
// 3. Verifies identifier matches expected format
// 4. Verifies user is owner in org_users table
