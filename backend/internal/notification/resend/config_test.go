package resend

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	transactional "github.com/trakrf/platform/backend/internal/services/email"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"NOTIFICATION_EMAIL_ENABLED", "NOTIFICATION_EMAIL_FROM", "RESEND_WEBHOOK_SECRET", "NOTIFICATION_EMAIL_TIMEOUT", "RESEND_API_KEY"} {
		t.Setenv(key, "")
	}
}

func enabledConfig() Config {
	return Config{Enabled: true, APIKey: "re_test_secret", From: "TrakRF <alerts@example.com>", WebhookSecret: "whsec_dGVzdC1zZWNyZXQ=", Timeout: 10 * time.Second}
}

func TestConfigDisabledWithTransactionalKey(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("RESEND_API_KEY", "re_transactional_secret")
	// Stale notification settings must not break a transactional-only deployment.
	t.Setenv("NOTIFICATION_EMAIL_TIMEOUT", "invalid")
	t.Setenv("NOTIFICATION_EMAIL_FROM", "invalid")
	config, err := ConfigFromEnv()
	require.NoError(t, err)
	require.False(t, config.Enabled)
	require.NoError(t, config.Validate())
	require.Equal(t, "re_transactional_secret", os.Getenv("RESEND_API_KEY"))
	require.NotNil(t, transactional.NewClient())
	t.Setenv("NOTIFICATION_EMAIL_ENABLED", "false")
	config, err = ConfigFromEnv()
	require.NoError(t, err)
	require.False(t, config.Enabled)
}

func TestConfigFromEnv(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("NOTIFICATION_EMAIL_ENABLED", "true")
	t.Setenv("RESEND_API_KEY", "re_test_secret")
	t.Setenv("NOTIFICATION_EMAIL_FROM", "TrakRF <alerts@example.com>")
	t.Setenv("RESEND_WEBHOOK_SECRET", "whsec_dGVzdC1zZWNyZXQ=")
	c, err := ConfigFromEnv()
	require.NoError(t, err)
	require.Equal(t, enabledConfig(), c)
	t.Setenv("NOTIFICATION_EMAIL_TIMEOUT", "250ms")
	c, err = ConfigFromEnv()
	require.NoError(t, err)
	require.Equal(t, 250*time.Millisecond, c.Timeout)
	for _, value := range []string{"0s", "-1s", "secret-invalid"} {
		t.Setenv("NOTIFICATION_EMAIL_TIMEOUT", value)
		_, err = ConfigFromEnv()
		require.Error(t, err)
		require.NotContains(t, err.Error(), value)
	}
	t.Setenv("NOTIFICATION_EMAIL_ENABLED", "secret-invalid")
	_, err = ConfigFromEnv()
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret-invalid")
}

func TestConfigRejectsIncompleteOrUnsafeEnabledValues(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*Config)
	}{
		{"key", func(c *Config) { c.APIKey = "" }},
		{"blank key", func(c *Config) { c.APIKey = "   " }},
		{"key newline", func(c *Config) { c.APIKey = "re_secret\r\nInjected: yes" }},
		{"sender", func(c *Config) { c.From = "" }},
		{"invalid sender", func(c *Config) { c.From = "secret-invalid" }},
		{"sender list", func(c *Config) { c.From = "a@example.com, b@example.com" }},
		{"sender newline", func(c *Config) { c.From = "secret\r\n <a@example.com>" }},
		{"missing domain", func(c *Config) { c.From = "a@localhost" }},
		{"webhook", func(c *Config) { c.WebhookSecret = "" }},
		{"webhook format", func(c *Config) { c.WebhookSecret = "secret-invalid" }},
		{"webhook encoding", func(c *Config) { c.WebhookSecret = "whsec_%%%" }},
		{"webhook empty", func(c *Config) { c.WebhookSecret = "whsec_" }},
		{"timeout", func(c *Config) { c.Timeout = 0 }},
		{"negative timeout", func(c *Config) { c.Timeout = -time.Second }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			c := enabledConfig()
			tc.mutate(&c)
			err := c.Validate()
			require.Error(t, err)
			require.False(t, strings.Contains(err.Error(), "secret"))
		})
	}
	require.NoError(t, Config{}.Validate())
}
