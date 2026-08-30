package twilio_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/twilio"
)

const (
	accountSID          = "AC1234567890"
	apiKeySID           = "SK1234567890"
	apiKeySecret        = "api-key-secret-value"
	authToken           = "auth-token-secret-value"
	messagingServiceSID = "MG1234567890"
	publicBaseURL       = "https://api.example.com"
)

func setCompleteConfig(t *testing.T) {
	t.Helper()
	t.Setenv("TWILIO_ACCOUNT_SID", accountSID)
	t.Setenv("TWILIO_API_KEY_SID", apiKeySID)
	t.Setenv("TWILIO_API_KEY_SECRET", apiKeySecret)
	t.Setenv("TWILIO_AUTH_TOKEN", authToken)
	t.Setenv("TWILIO_MESSAGING_SERVICE_SID", messagingServiceSID)
	t.Setenv("TWILIO_PUBLIC_BASE_URL", publicBaseURL)
}

// This fails if an empty environment starts enabling Twilio or returns credentials.
func TestConfigFromEnv_AllEmptyDisablesTwilio(t *testing.T) {
	for _, name := range []string{
		"TWILIO_ACCOUNT_SID",
		"TWILIO_API_KEY_SID",
		"TWILIO_API_KEY_SECRET",
		"TWILIO_AUTH_TOKEN",
		"TWILIO_MESSAGING_SERVICE_SID",
		"TWILIO_PUBLIC_BASE_URL",
	} {
		t.Setenv(name, "")
	}

	config, err := twilio.ConfigFromEnv()

	require.NoError(t, err)
	require.Equal(t, twilio.Config{}, config)
	require.False(t, config.Enabled())
}

// This fails if a fully configured integration loses or changes a configured value.
func TestConfigFromEnv_CompleteConfigEnablesTwilio(t *testing.T) {
	setCompleteConfig(t)

	config, err := twilio.ConfigFromEnv()

	require.NoError(t, err)
	require.Equal(t, twilio.Config{
		AccountSID:          accountSID,
		APIKeySID:           apiKeySID,
		APIKeySecret:        apiKeySecret,
		AuthToken:           authToken,
		MessagingServiceSID: messagingServiceSID,
		PublicBaseURL:       publicBaseURL,
	}, config)
	require.True(t, config.Enabled())
}

// This fails if a partially configured integration is enabled or leaks secrets in its error.
func TestConfigFromEnv_PartialConfigFailsClosedWithoutLeakingSecrets(t *testing.T) {
	for _, missing := range []string{
		"TWILIO_ACCOUNT_SID",
		"TWILIO_API_KEY_SID",
		"TWILIO_API_KEY_SECRET",
		"TWILIO_AUTH_TOKEN",
		"TWILIO_MESSAGING_SERVICE_SID",
		"TWILIO_PUBLIC_BASE_URL",
	} {
		t.Run(missing, func(t *testing.T) {
			setCompleteConfig(t)
			t.Setenv(missing, "")

			config, err := twilio.ConfigFromEnv()

			require.Error(t, err)
			require.Equal(t, twilio.Config{}, config)
			require.False(t, config.Enabled())
			require.NotContains(t, err.Error(), apiKeySecret)
			require.NotContains(t, err.Error(), authToken)
		})
	}
}

// This fails if a callback URL can use insecure HTTP in a configured integration.
func TestConfigFromEnv_RejectsNonHTTPSPublicBaseURL(t *testing.T) {
	setCompleteConfig(t)
	t.Setenv("TWILIO_PUBLIC_BASE_URL", "http://api.example.com")

	config, err := twilio.ConfigFromEnv()

	require.Error(t, err)
	require.Equal(t, twilio.Config{}, config)
	require.False(t, config.Enabled())
	require.False(t, strings.Contains(err.Error(), "http://api.example.com"))
}
