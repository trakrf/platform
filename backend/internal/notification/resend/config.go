// Package resend implements the Resend notification email boundary.
package resend

import (
	"encoding/base64"
	"errors"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultTimeout = 10 * time.Second

// Config is independent of the existing transactional email service. Possessing
// a RESEND_API_KEY alone never enables notification traffic or callback routes.
type Config struct {
	Enabled       bool
	APIKey        string
	From          string
	WebhookSecret string
	Timeout       time.Duration
}

// ConfigFromEnv ignores inactive provider settings when explicitly disabled or
// unset, so adding notification configuration cannot break transactional email.
func ConfigFromEnv() (Config, error) {
	rawEnabled := os.Getenv("NOTIFICATION_EMAIL_ENABLED")
	if rawEnabled == "" {
		return Config{}, nil
	}
	enabled, err := strconv.ParseBool(rawEnabled)
	if err != nil {
		return Config{}, errors.New("NOTIFICATION_EMAIL_ENABLED must be a boolean")
	}
	if !enabled {
		return Config{}, nil
	}
	c := Config{
		Enabled:       true,
		APIKey:        os.Getenv("RESEND_API_KEY"),
		From:          os.Getenv("NOTIFICATION_EMAIL_FROM"),
		WebhookSecret: os.Getenv("RESEND_WEBHOOK_SECRET"),
		Timeout:       defaultTimeout,
	}
	if raw := os.Getenv("NOTIFICATION_EMAIL_TIMEOUT"); raw != "" {
		c.Timeout, err = time.ParseDuration(raw)
		if err != nil {
			return Config{}, errors.New("NOTIFICATION_EMAIL_TIMEOUT must be a positive duration")
		}
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.APIKey) == "" || strings.ContainsAny(c.APIKey, "\r\n") || strings.TrimSpace(c.APIKey) != c.APIKey {
		return errors.New("notification email API key is required and must not contain surrounding whitespace or line breaks")
	}
	if !validAddress(c.From, true) {
		return errors.New("notification email sender must be a single valid mailbox")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(c.WebhookSecret, "whsec_"))
	if !strings.HasPrefix(c.WebhookSecret, "whsec_") || err != nil || len(key) == 0 || strings.ContainsAny(c.WebhookSecret, "\r\n") {
		return errors.New("notification email webhook signing key must use whsec_ followed by base64")
	}
	if c.Timeout <= 0 {
		return errors.New("notification email timeout must be positive")
	}
	return nil
}

func validAddress(raw string, allowName bool) bool {
	if strings.ContainsAny(raw, "\r\n") {
		return false
	}
	address, err := mail.ParseAddress(raw)
	if err != nil || (!allowName && raw != address.Address) {
		return false
	}
	at := strings.LastIndexByte(address.Address, '@')
	return at > 0 && strings.Contains(address.Address[at+1:], ".")
}
