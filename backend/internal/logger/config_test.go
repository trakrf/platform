package logger

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectEnvironment(t *testing.T) {
	tests := []struct {
		name     string
		appEnv   string
		expected Environment
	}{
		{name: "unset is local development", appEnv: "", expected: EnvDev},
		{name: "dev", appEnv: "dev", expected: EnvDev},
		{name: "development", appEnv: "development", expected: EnvDev},
		{name: "local", appEnv: "local", expected: EnvDev},
		{name: "test harness", appEnv: "test", expected: EnvDev},
		{name: "staging", appEnv: "staging", expected: EnvStaging},
		{name: "preview runs the staging profile", appEnv: "preview", expected: EnvStaging},
		{name: "prod (the chart's value)", appEnv: "prod", expected: EnvProd},
		{name: "production", appEnv: "production", expected: EnvProd},
		{name: "case-insensitive", appEnv: "PROD", expected: EnvProd},
		{name: "unrecognized fails closed to prod", appEnv: "whitelabel-acme", expected: EnvProd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", tt.appEnv)

			assert.Equal(t, tt.expected, DetectEnvironment())
		})
	}
}

// ENVIRONMENT was the logger's old selector; nothing sets it, and it must not
// override APP_ENV (that mismatch ran prod on the dev profile).
func TestDetectEnvironmentIgnoresENVIRONMENT(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("ENVIRONMENT", "dev")

	assert.Equal(t, EnvProd, DetectEnvironment())
}

func TestDetectServiceName(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "Custom service name from env",
			envValue: "custom-service",
			expected: "custom-service",
		},
		{
			name:     "Empty service name defaults to platform-backend",
			envValue: "",
			expected: "platform-backend",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv("SERVICE_NAME", tt.envValue)
				defer os.Unsetenv("SERVICE_NAME")
			}

			result := DetectServiceName()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		environment string
		expected    Config
	}{
		{
			name:        "Development configuration",
			version:     "1.0.0",
			environment: "dev",
			expected: Config{
				Environment:    EnvDev,
				ServiceName:    "platform-backend",
				Level:          "debug",
				Format:         "console",
				IncludeStack:   true,
				IncludeCaller:  true,
				ColorOutput:    true,
				SanitizeEmails: false,
				SanitizeIPs:    false,
				MaxBodySize:    1000,
				Version:        "1.0.0",
			},
		},
		{
			name:        "Staging configuration",
			version:     "2.0.0",
			environment: "staging",
			expected: Config{
				Environment:    EnvStaging,
				ServiceName:    "platform-backend",
				Level:          "info",
				Format:         "json",
				IncludeStack:   false,
				IncludeCaller:  false,
				ColorOutput:    false,
				SanitizeEmails: true,
				SanitizeIPs:    true,
				MaxBodySize:    0,
				Version:        "2.0.0",
			},
		},
		{
			name:        "Production configuration",
			version:     "3.0.0",
			environment: "prod",
			expected: Config{
				Environment:    EnvProd,
				ServiceName:    "platform-backend",
				Level:          "info",
				Format:         "json",
				IncludeStack:   false,
				IncludeCaller:  false,
				ColorOutput:    false,
				SanitizeEmails: true,
				SanitizeIPs:    true,
				MaxBodySize:    0,
				Version:        "3.0.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", tt.environment)

			result := NewConfig(tt.version)
			assert.Equal(t, tt.expected, *result)
		})
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "Returns env value when set",
			key:          "TEST_KEY",
			defaultValue: "default",
			envValue:     "custom",
			expected:     "custom",
		},
		{
			name:         "Returns default when env not set",
			key:          "TEST_KEY",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}

			result := getEnvOrDefault(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetBoolEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue bool
		envValue     string
		expected     bool
	}{
		{
			name:         "Returns true when env is 'true'",
			key:          "TEST_BOOL",
			defaultValue: false,
			envValue:     "true",
			expected:     true,
		},
		{
			name:         "Returns true when env is '1'",
			key:          "TEST_BOOL",
			defaultValue: false,
			envValue:     "1",
			expected:     true,
		},
		{
			name:         "Returns false when env is 'false'",
			key:          "TEST_BOOL",
			defaultValue: true,
			envValue:     "false",
			expected:     false,
		},
		{
			name:         "Returns false when env is '0'",
			key:          "TEST_BOOL",
			defaultValue: true,
			envValue:     "0",
			expected:     false,
		},
		{
			name:         "Returns default when env not set",
			key:          "TEST_BOOL",
			defaultValue: true,
			envValue:     "",
			expected:     true,
		},
		{
			name:         "Returns false when env is invalid (not empty)",
			key:          "TEST_BOOL",
			defaultValue: true,
			envValue:     "invalid",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			result := getBoolEnv(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetIntEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue int
		envValue     string
		expected     int
	}{
		{
			name:         "Returns int value when env is valid",
			key:          "TEST_INT",
			defaultValue: 10,
			envValue:     "42",
			expected:     42,
		},
		{
			name:         "Returns default when env not set",
			key:          "TEST_INT",
			defaultValue: 10,
			envValue:     "",
			expected:     10,
		},
		{
			name:         "Returns default when env is invalid",
			key:          "TEST_INT",
			defaultValue: 10,
			envValue:     "invalid",
			expected:     10,
		},
		{
			name:         "Returns negative int when env is negative",
			key:          "TEST_INT",
			defaultValue: 10,
			envValue:     "-5",
			expected:     -5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			result := getIntEnv(tt.key, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}
