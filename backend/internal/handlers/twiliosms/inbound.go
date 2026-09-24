package twiliosms

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/trakrf/platform/backend/internal/notification/twilio"
)

// Inbound receives a signature-verified Twilio inbound-message callback.
func (h *Handler) Inbound(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	result := twilio.CallbackMalformed
	defer func() {
		h.recordCallback(twilio.CallbackInbound, result, startedAt)
	}()

	form, err := h.verifiedForm(w, r)
	if err != nil {
		if errors.Is(err, errInvalidSignature) {
			result = twilio.CallbackInvalidSignature
			http.Error(w, "invalid callback signature", http.StatusForbidden)
			return
		}
		http.Error(w, "invalid callback", http.StatusBadRequest)
		return
	}

	keyword, recognized, err := normalizedConsentKeyword(form)
	if err != nil {
		http.Error(w, "invalid callback", http.StatusBadRequest)
		return
	}
	if !recognized {
		result = twilio.CallbackAccepted
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
		result = twilio.CallbackConsumerFailure
		http.Error(w, "callback consumer unavailable", http.StatusInternalServerError)
		return
	}

	result = twilio.CallbackConsumerFailure
	if err := h.consumer.HandleKeyword(r.Context(), sms.InboundKeyword{
		ProviderMessageID: messageID,
		FromE164:          from,
		ToE164:            to,
		Keyword:           keyword,
		ReceivedAt:        h.currentTime(),
	}); err != nil {
		result = twilio.CallbackConsumerFailure
		http.Error(w, "callback consumer failed", http.StatusInternalServerError)
		return
	}

	result = twilio.CallbackAccepted
	w.WriteHeader(http.StatusNoContent)
}

// Provider classifications are meaningful only after signature validation.
// HELP must not fall through to a contradictory body keyword.
func normalizedConsentKeyword(form url.Values) (string, bool, error) {
	if form.Has("OptOutType") {
		switch form.Get("OptOutType") {
		case "STOP", "START":
			return form.Get("OptOutType"), true, nil
		case "HELP":
			return "", false, nil
		default:
			return "", false, errMalformedForm
		}
	}
	keyword, recognized := normalizedInboundKeyword(form.Get("Body"))
	return keyword, recognized, nil
}

func normalizedInboundKeyword(body string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(body)) {
	case "STOP", "CANCEL", "UNSUBSCRIBE", "END", "QUIT", "STOPALL", "REVOKE", "OPTOUT":
		return "STOP", true
	case "START", "UNSTOP", "YES":
		return "START", true
	default:
		return "", false
	}
}
