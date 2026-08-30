// Package twiliosms handles signature-verified Twilio SMS callbacks.
package twiliosms

import (
	"errors"
	"net/url"
	"time"

	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/trakrf/platform/backend/internal/notification/twilio"
	"github.com/twilio/twilio-go/client"
)

// Handler provides the shared boundary for Twilio SMS callbacks.
type Handler struct {
	consumer      sms.CallbackConsumer
	publicBaseURL string
	validator     client.RequestValidator
	now           func() time.Time
}

// NewHandler builds a Twilio callback handler only from a complete Twilio
// configuration with a canonical public HTTPS origin.
func NewHandler(config twilio.Config, consumer sms.CallbackConsumer) (*Handler, error) {
	if !config.Enabled() {
		return nil, errors.New("Twilio callback configuration is incomplete")
	}
	if !validCallbackPublicBaseURL(config.PublicBaseURL) {
		return nil, errors.New("Twilio callback public base URL must be a canonical HTTPS origin")
	}

	return &Handler{
		consumer:      consumer,
		publicBaseURL: config.PublicBaseURL,
		validator:     client.NewRequestValidator(config.AuthToken),
		now:           time.Now,
	}, nil
}

func validCallbackPublicBaseURL(raw string) bool {
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
