//go:build integration

package storage_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/email"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/migrations"
)

func TestEmailCallbackPersistence(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	consumer := db.Store.EmailCallbackConsumer()
	at := time.Now().UTC().Truncate(time.Microsecond)
	event := email.CallbackEvent{Provider: "resend", ProviderEventID: "event-1", ProviderMessageID: "uncorrelated-message", Type: email.EventDelivered, OccurredAt: at.Add(-time.Hour), ReceivedAt: at}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- consumer.HandleEvent(ctx, event) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	// Replay must preserve the original row, even with different receipt time
	// or conflicting content. Deduplication uses provider event identity only.
	replay := event
	replay.ReceivedAt = at.Add(time.Hour)
	replay.Type = email.EventFailed
	require.NoError(t, db.Store.EmailCallbackConsumer().HandleEvent(ctx, replay))
	var stored email.CallbackEvent
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT provider, provider_event_id, provider_message_id, event_type, occurred_at, received_at FROM trakrf.email_callback_events`).Scan(&stored.Provider, &stored.ProviderEventID, &stored.ProviderMessageID, &stored.Type, &stored.OccurredAt, &stored.ReceivedAt))
	require.Equal(t, event.Provider, stored.Provider)
	require.Equal(t, event.ProviderEventID, stored.ProviderEventID)
	require.Equal(t, event.ProviderMessageID, stored.ProviderMessageID)
	require.Equal(t, event.Type, stored.Type)
	require.True(t, event.OccurredAt.Equal(stored.OccurredAt))
	require.True(t, event.ReceivedAt.Equal(stored.ReceivedAt))
	// Distinct events for the same message, including older ones, all survive.
	for i, kind := range []email.EventType{email.EventFailed, email.EventBounced, email.EventComplained} {
		next := event
		next.ProviderEventID = string(rune('a' + i))
		next.Type = kind
		next.OccurredAt = at.Add(-2 * time.Hour)
		require.NoError(t, consumer.HandleEvent(ctx, next))
	}
	otherProvider := event
	otherProvider.Provider = "another-provider"
	require.NoError(t, consumer.HandleEvent(ctx, otherProvider))
	var count int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.email_callback_events`).Scan(&count))
	require.Equal(t, 5, count)
	// Canceled writes and real database failures must never acknowledge success.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.EqualError(t, consumer.HandleEvent(canceled, event), "email callback persistence failed")
	_, err := db.AdminPool.Exec(ctx, `REVOKE INSERT ON trakrf.email_callback_events FROM trakrf_test_app`)
	require.NoError(t, err)
	require.EqualError(t, consumer.HandleEvent(ctx, event), "email callback persistence failed")
	_, err = db.AdminPool.Exec(ctx, `GRANT INSERT ON trakrf.email_callback_events TO trakrf_test_app`)
	require.NoError(t, err)
	require.NoError(t, consumer.HandleEvent(ctx, event))
}

func TestEmailCallbackMigrationRoundTrip(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	down, err := migrations.FS.ReadFile("000042_email_callback_events.down.sql")
	require.NoError(t, err)
	_, err = db.AdminPool.Exec(ctx, string(down))
	require.NoError(t, err)
	var absent bool
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT to_regclass('trakrf.email_callback_events') IS NULL`).Scan(&absent))
	require.True(t, absent)
	up, err := migrations.FS.ReadFile("000042_email_callback_events.up.sql")
	require.NoError(t, err)
	_, err = db.AdminPool.Exec(ctx, string(up))
	require.NoError(t, err)
	var count int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.email_callback_events`).Scan(&count))
	require.Zero(t, count)
}
