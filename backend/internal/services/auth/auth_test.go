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
		{"simple name", "My Company", "my-company"},
		{"special chars", "ACME Corp!", "acme-corp"},
		{"multiple spaces", "Test  Multiple   Spaces", "test-multiple-spaces"},
		{"numbers", "Company123", "company123"},
		{"hyphens already", "my-company", "my-company"},
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
