// Package twilio implements the Twilio SMS provider boundary.
package twilio

import (
	"errors"
	"net/url"
	"os"
)

// Config contains the credentials and canonical public callback origin for Twilio SMS.
// PublicBaseURL must not include a trailing slash, path, query, fragment, or userinfo.
type Config struct {
	AccountSID          string
	APIKeySID           string
	APIKeySecret        string
	AuthToken           string
	MessagingServiceSID string
	PublicBaseURL       string
}

// ConfigFromEnv reads the complete Twilio configuration. An entirely unset
// configuration disables Twilio; any partially configured state is rejected.
func ConfigFromEnv() (Config, error) {
	config := Config{
		AccountSID:          os.Getenv("TWILIO_ACCOUNT_SID"),
		APIKeySID:           os.Getenv("TWILIO_API_KEY_SID"),
		APIKeySecret:        os.Getenv("TWILIO_API_KEY_SECRET"),
		AuthToken:           os.Getenv("TWILIO_AUTH_TOKEN"),
		MessagingServiceSID: os.Getenv("TWILIO_MESSAGING_SERVICE_SID"),
		PublicBaseURL:       os.Getenv("TWILIO_PUBLIC_BASE_URL"),
	}

	if config == (Config{}) {
		return config, nil
	}
	if !config.Enabled() {
		return Config{}, errors.New("Twilio configuration is incomplete")
	}
	if !validPublicBaseURL(config.PublicBaseURL) {
		return Config{}, errors.New("Twilio public base URL must be a canonical HTTPS origin")
	}

	return config, nil
}

// Enabled reports whether every value required to use Twilio is configured.
func (c Config) Enabled() bool {
	return c.AccountSID != "" &&
		c.APIKeySID != "" &&
		c.APIKeySecret != "" &&
		c.AuthToken != "" &&
		c.MessagingServiceSID != "" &&
		c.PublicBaseURL != ""
}

func validPublicBaseURL(raw string) bool {
	parsed, err := url.ParseRequestURI(raw)
	return err == nil &&
		parsed.Scheme == "https" &&
		parsed.Host != "" &&
		parsed.User == nil &&
		parsed.Path == "" &&
		parsed.RawPath == "" &&
		parsed.RawQuery == "" &&
		parsed.Fragment == "" &&
		!parsed.ForceQuery &&
		parsed.Opaque == ""
}
