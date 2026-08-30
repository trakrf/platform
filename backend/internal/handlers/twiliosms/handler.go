// Package twiliosms handles signature-verified Twilio SMS callbacks.
package twiliosms

import (
	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/trakrf/platform/backend/internal/notification/twilio"
	"github.com/twilio/twilio-go/client"
)

// Handler provides the shared boundary for Twilio SMS callbacks.
type Handler struct {
	consumer      sms.CallbackConsumer
	publicBaseURL string
	validator     client.RequestValidator
}

// NewHandler builds a Twilio callback handler. Config must have already passed
// the notification/twilio configuration validation.
func NewHandler(config twilio.Config, consumer sms.CallbackConsumer) *Handler {
	return &Handler{
		consumer:      consumer,
		publicBaseURL: config.PublicBaseURL,
		validator:     client.NewRequestValidator(config.AuthToken),
	}
}
