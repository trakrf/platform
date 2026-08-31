package twiliosms

import (
	"errors"
	"net/http"
	"strings"

	"github.com/trakrf/platform/backend/internal/notification/sms"
)

// Inbound receives a signature-verified Twilio inbound-message callback.
func (h *Handler) Inbound(w http.ResponseWriter, r *http.Request) {
	form, err := h.verifiedForm(w, r)
	if err != nil {
		if errors.Is(err, errInvalidSignature) {
			http.Error(w, "invalid callback signature", http.StatusForbidden)
			return
		}
		http.Error(w, "invalid callback", http.StatusBadRequest)
		return
	}

	keyword, recognized := normalizedInboundKeyword(form.Get("Body"))
	if !recognized {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	messageID := form.Get("MessageSid")
	from := form.Get("From")
	to := form.Get("To")
	if messageID == "" || from == "" || to == "" {
		http.Error(w, "invalid callback", http.StatusBadRequest)
		return
	}
	if nilCallbackConsumer(h.consumer) {
		http.Error(w, "callback consumer unavailable", http.StatusInternalServerError)
		return
	}

	if err := h.consumer.HandleKeyword(r.Context(), sms.InboundKeyword{
		ProviderMessageID: messageID,
		FromE164:          from,
		ToE164:            to,
		Keyword:           keyword,
		ReceivedAt:        h.currentTime(),
	}); err != nil {
		http.Error(w, "callback consumer failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func normalizedInboundKeyword(body string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(body)) {
	case "STOP", "CANCEL", "UNSUBSCRIBE", "END", "QUIT", "STOPALL", "REVOKE", "OPTOUT":
		return "STOP", true
	case "START", "UNSTOP":
		return "START", true
	default:
		return "", false
	}
}
