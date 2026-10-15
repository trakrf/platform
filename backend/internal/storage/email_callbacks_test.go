package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/email"
)

func TestEmailCallbackRejectsInvalidEventsBeforeStorage(t *testing.T) {
	consumer := (&Storage{}).EmailCallbackConsumer()
	for _, mutate := range []func(*email.CallbackEvent){
		func(e *email.CallbackEvent) { e.Provider = " " },
		func(e *email.CallbackEvent) { e.ProviderEventID = " " },
		func(e *email.CallbackEvent) { e.ProviderMessageID = " " },
		func(e *email.CallbackEvent) { e.Type = "unknown" },
		func(e *email.CallbackEvent) { e.OccurredAt = time.Time{} },
		func(e *email.CallbackEvent) { e.ReceivedAt = time.Time{} },
	} {
		event := email.CallbackEvent{Provider: "resend", ProviderEventID: "event", ProviderMessageID: "message", Type: email.EventDelivered, OccurredAt: time.Now(), ReceivedAt: time.Now()}
		mutate(&event)
		require.EqualError(t, consumer.HandleEvent(context.Background(), event), "invalid email callback event")
	}
}
