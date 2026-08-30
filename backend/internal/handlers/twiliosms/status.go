package twiliosms

import (
	"errors"
	"net/http"
	"reflect"
	"time"

	"github.com/trakrf/platform/backend/internal/notification/sms"
)

// Status receives a signature-verified Twilio delivery-status callback.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	form, err := h.verifiedForm(w, r)
	if err != nil {
		if errors.Is(err, errInvalidSignature) {
			http.Error(w, "invalid callback signature", http.StatusForbidden)
			return
		}
		http.Error(w, "invalid callback", http.StatusBadRequest)
		return
	}

	messageID := form.Get("MessageSid")
	status := form.Get("MessageStatus")
	if messageID == "" || !knownDeliveryStatus(status) {
		http.Error(w, "invalid callback", http.StatusBadRequest)
		return
	}
	if nilCallbackConsumer(h.consumer) {
		http.Error(w, "callback consumer unavailable", http.StatusInternalServerError)
		return
	}

	if err := h.consumer.HandleStatus(r.Context(), sms.ProviderStatus{
		ProviderMessageID: messageID,
		Status:            status,
		ErrorCode:         form.Get("ErrorCode"),
		OccurredAt:        h.currentTime(),
	}); err != nil {
		http.Error(w, "callback consumer failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func knownDeliveryStatus(status string) bool {
	switch status {
	case "queued", "sent", "delivered", "undelivered", "failed":
		return true
	default:
		return false
	}
}

func (h *Handler) currentTime() time.Time {
	if h.now == nil {
		return time.Now().UTC()
	}
	return h.now().UTC()
}

func nilCallbackConsumer(consumer sms.CallbackConsumer) bool {
	if consumer == nil {
		return true
	}

	value := reflect.ValueOf(consumer)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
