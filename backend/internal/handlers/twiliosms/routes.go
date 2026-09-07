package twiliosms

import "github.com/go-chi/chi/v5"

const (
	statusCallbackPath  = "/api/v1/notifications/twilio/status"
	inboundCallbackPath = "/api/v1/notifications/twilio/inbound"
)

// RegisterRoutes registers Twilio's public, signature-verified callback
// endpoints. The production server attaches them only when it has a durable
// callback consumer to inject.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post(statusCallbackPath, h.Status)
	r.Post(inboundCallbackPath, h.Inbound)
}
