package storage

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/trakrf/platform/backend/internal/notification/email"
)

// EmailCallbackConsumer records verified platform events without tenant context.
// Ownership resolution belongs to a downstream workflow, not the callback inbox.
func (s *Storage) EmailCallbackConsumer() email.CallbackConsumer {
	return &emailCallbackConsumer{store: s}
}

type emailCallbackConsumer struct{ store *Storage }

func (c *emailCallbackConsumer) HandleEvent(ctx context.Context, event email.CallbackEvent) error {
	if strings.TrimSpace(event.Provider) == "" || strings.TrimSpace(event.ProviderEventID) == "" ||
		strings.TrimSpace(event.ProviderMessageID) == "" || event.OccurredAt.IsZero() || event.ReceivedAt.IsZero() {
		return errors.New("invalid email callback event")
	}
	switch event.Type {
	case email.EventDelivered, email.EventFailed, email.EventBounced, email.EventComplained:
	default:
		return errors.New("invalid email callback event")
	}
	// Stay within the HTTP server's write deadline during database contention.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// One autocommitted insert arbitrates concurrent replicas. Replays never
	// overwrite the first receipt or discard distinct events for the same message.
	_, err := c.store.pool.Exec(ctx, `INSERT INTO trakrf.email_callback_events
  (provider, provider_event_id, provider_message_id, event_type, occurred_at, received_at)
  VALUES ($1, $2, $3, $4, $5, $6)
  ON CONFLICT (provider, provider_event_id) DO NOTHING`,
		event.Provider, event.ProviderEventID, event.ProviderMessageID, event.Type, event.OccurredAt.UTC(), event.ReceivedAt.UTC())
	if err != nil {
		// Database errors can contain event identities. Never expose the cause.
		return errors.New("email callback persistence failed")
	}
	return nil
}
