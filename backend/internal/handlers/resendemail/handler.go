// Package resendemail receives signature-verified Resend notification callbacks.
package resendemail

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	sdk "github.com/resend/resend-go/v2"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/notification/resend"
)

const maxBodyBytes = 256 * 1024

// Handler verifies raw bytes before decoding and acknowledges supported events
// only after durable consumption. Runtime route mounting is a separate concern.
type Handler struct {
	consumer email.CallbackConsumer
	verifier sdk.WebhooksSvc
	secret   string
}

func NewHandler(config resend.Config, consumer email.CallbackConsumer) (*Handler, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, errors.New("notification email callbacks disabled")
	}
	if nilConsumer(consumer) {
		return nil, errors.New("email callback consumer required")
	}
	// Verify is entirely local; no API key or network operation is required.
	return &Handler{consumer: consumer, verifier: sdk.NewClient("").Webhooks, secret: config.WebhookSecret}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	receivedAt := time.Now().UTC()
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "callback too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "invalid callback", http.StatusBadRequest)
		}
		return
	}
	// The pinned SDK compares signature bytes but ignores version labels. Pass
	// only supported v1 entries, retaining multiple signatures for key rotation.
	var signatures []string
	for _, signature := range strings.Fields(r.Header.Get("svix-signature")) {
		if strings.HasPrefix(signature, "v1,") {
			signatures = append(signatures, signature)
		}
	}
	eventID := r.Header.Get("svix-id")
	if err = h.verifier.Verify(&sdk.VerifyWebhookOptions{
		Payload: string(body), WebhookSecret: h.secret,
		Headers: sdk.WebhookHeaders{Id: eventID, Timestamp: r.Header.Get("svix-timestamp"), Signature: strings.Join(signatures, " ")},
	}); err != nil {
		http.Error(w, "invalid callback signature", http.StatusForbidden)
		return
	}
	var envelope struct {
		Type      string          `json:"type"`
		CreatedAt string          `json:"created_at"`
		Data      json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil || strings.TrimSpace(envelope.Type) == "" {
		http.Error(w, "invalid callback", http.StatusBadRequest)
		return
	}
	kind, ok := eventType(envelope.Type)
	if !ok {
		// Authenticated future/unrelated event types do not need retries. Their
		// data schema may differ; do not require email-specific fields.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var data struct {
		EmailID string `json:"email_id"`
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, envelope.CreatedAt)
	if err != nil || occurredAt.IsZero() || json.Unmarshal(envelope.Data, &data) != nil ||
		strings.TrimSpace(data.EmailID) == "" || strings.TrimSpace(eventID) == "" {
		http.Error(w, "invalid callback", http.StatusBadRequest)
		return
	}
	if err = h.consumer.HandleEvent(r.Context(), email.CallbackEvent{
		Provider: "resend", ProviderEventID: eventID, ProviderMessageID: data.EmailID,
		Type: kind, OccurredAt: occurredAt.UTC(), ReceivedAt: receivedAt,
	}); err != nil {
		http.Error(w, "callback persistence failed", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func eventType(providerType string) (email.EventType, bool) {
	switch providerType {
	case "email.delivered":
		return email.EventDelivered, true
	case "email.failed":
		return email.EventFailed, true
	case "email.bounced":
		return email.EventBounced, true
	case "email.complained":
		return email.EventComplained, true
	default:
		return "", false
	}
}

func nilConsumer(consumer email.CallbackConsumer) bool {
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
