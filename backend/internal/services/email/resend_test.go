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
