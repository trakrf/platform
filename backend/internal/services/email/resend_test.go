package email

import (
	"os"
	"strings"
	"testing"
)

func TestGetEmailPrefix(t *testing.T) {
	tests := []struct {
		name     string
		appEnv   string
		expected string
	}{
		{"empty env", "", "[TrakRF]"},
		{"production", "production", "[TrakRF]"},
		{"prod", "prod", "[TrakRF]"},
		{"preview", "preview", "[TrakRF Preview]"},
		{"staging", "staging", "[TrakRF Staging]"},
		{"dev", "dev", "[TrakRF Dev]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("APP_ENV", tt.appEnv)
			defer os.Unsetenv("APP_ENV")

			result := getEmailPrefix()
			if result != tt.expected {
				t.Errorf("getEmailPrefix() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetEnvironmentNotice(t *testing.T) {
	tests := []struct {
		name          string
		appEnv        string
		shouldBeEmpty bool
		contains      string
	}{
		{"empty env", "", true, ""},
		{"production", "production", true, ""},
		{"prod", "prod", true, ""},
		{"preview", "preview", false, "Preview environment"},
		{"staging", "staging", false, "Staging environment"},
		{"dev", "dev", false, "Dev environment"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("APP_ENV", tt.appEnv)
			defer os.Unsetenv("APP_ENV")

			result := getEnvironmentNotice()
			if tt.shouldBeEmpty && result != "" {
				t.Errorf("getEnvironmentNotice() = %q, want empty", result)
			}
			if !tt.shouldBeEmpty && result == "" {
				t.Errorf("getEnvironmentNotice() = empty, want non-empty containing %q", tt.contains)
			}
			if !tt.shouldBeEmpty && !strings.Contains(result, tt.contains) {
				t.Errorf("getEnvironmentNotice() = %q, want containing %q", result, tt.contains)
			}
		})
	}
}

func TestIsReservedTestRecipient(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected bool
	}{
		{"example.com", "fixture@example.com", true},
		{"example.net", "fixture@example.net", true},
		{"example.org", "fixture@example.org", true},
		{".test TLD", "fixture@foo.test", true},
		{".invalid TLD", "fixture@foo.invalid", true},
		{".example TLD", "fixture@foo.example", true},
		{"case-insensitive domain", "Fixture@Example.COM", true},
		{"subdomain of example.com", "fixture@mail.example.com", true},
		{"trakrf.id", "user@trakrf.id", false},
		{"gmail", "user@gmail.com", false},
		{"gmail alias", "miks2u+t2@gmail.com", false},
		{"customer domain", "user@acme.co", false},
		{"empty", "", false},
		{"no at-sign", "not-an-email", false},
		{"trailing whitespace", "fixture@example.com  ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isReservedTestRecipient(tt.addr); got != tt.expected {
				t.Errorf("isReservedTestRecipient(%q) = %v, want %v", tt.addr, got, tt.expected)
			}
		})
	}
}

func TestSendInvitationEmail_StubsReservedDomain(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "invalid-key-should-never-be-used")
	c := NewClient()

	if err := c.SendInvitationEmail(
		"fixture@example.com",
		"Test Org",
		"Inviter Name",
		"member",
		"token-xyz",
		"https://app.preview.trakrf.id",
	); err != nil {
		t.Fatalf("expected nil error for reserved recipient, got %v", err)
	}
}

func TestSendPasswordResetEmail_StubsReservedDomain(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "invalid-key-should-never-be-used")
	c := NewClient()

	if err := c.SendPasswordResetEmail(
		"fixture@example.com",
		"https://app.preview.trakrf.id/#reset-password",
		"token-xyz",
	); err != nil {
		t.Fatalf("expected nil error for reserved recipient, got %v", err)
	}
}
