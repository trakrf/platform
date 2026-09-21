package storage

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"github.com/trakrf/platform/backend/internal/notification/sms"
)

// SMSCallbackConsumer binds verified callbacks to the configured provider
// account/service. These platform events have no authenticated tenant context;
// downstream notification workflows resolve tenant ownership separately.
func (s *Storage) SMSCallbackConsumer(accountSID, messagingServiceSID string) sms.CallbackConsumer {
	return &smsCallbackConsumer{store: s, accountSID: accountSID, messagingServiceSID: messagingServiceSID}
}

type smsCallbackConsumer struct {
	store                           *Storage
	accountSID, messagingServiceSID string
}

type smsCallbackPayload struct {
	Status    string `json:"status,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Keyword   string `json:"keyword,omitempty"`
}

func (c *smsCallbackConsumer) HandleStatus(ctx context.Context, event sms.ProviderStatus) error {
	return c.save(ctx, "status", event.ProviderMessageID, event.OccurredAt, smsCallbackPayload{
		Status: event.Status, ErrorCode: event.ErrorCode,
	})
}

func (c *smsCallbackConsumer) HandleKeyword(ctx context.Context, event sms.InboundKeyword) error {
	return c.save(ctx, "keyword", event.ProviderMessageID, event.ReceivedAt, smsCallbackPayload{
		From: event.FromE164, To: event.ToE164, Keyword: event.Keyword,
	})
}

func (c *smsCallbackConsumer) save(ctx context.Context, kind, messageID string, receivedAt time.Time, event smsCallbackPayload) error {
	// Finish within the HTTP server's write deadline even during DB contention.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	payload, err := json.Marshal(event)
	if err != nil {
		return errors.New("SMS callback encoding failed")
	}
	// Exclude local receipt time: Twilio retries the same provider event with a
	// new HTTP request. Preserve distinct lifecycle states and error codes.
	identity, err := json.Marshal([]any{c.accountSID, c.messagingServiceSID, kind, messageID, json.RawMessage(payload)})
	if err != nil {
		return errors.New("SMS callback encoding failed")
	}
	eventKey := sha256.Sum256(identity)
	_, err = c.store.pool.Exec(ctx, `INSERT INTO trakrf.sms_callback_events
		(event_key, account_sid, messaging_service_sid, event_type, provider_message_id, payload, received_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7)
		ON CONFLICT (event_key) DO NOTHING`,
		eventKey[:], c.accountSID, c.messagingServiceSID, kind, messageID, string(payload), receivedAt.UTC())
	if err != nil {
		// SQL/provider errors may contain callback identities or phone numbers.
		return errors.New("SMS callback persistence failed")
	}
	return nil
}
