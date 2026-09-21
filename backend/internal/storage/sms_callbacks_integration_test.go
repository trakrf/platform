//go:build integration

package storage_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/notification/sms"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// Callback acknowledgement must correspond to a committed row. Retries and
// concurrent replicas must not duplicate events or overwrite later statuses.
func TestSMSCallbacks_PersistAndDeduplicate(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	consumer := db.Store.SMSCallbackConsumer("ACtest", "MGtest")
	event := sms.ProviderStatus{ProviderMessageID: "SMtest", Status: "sent", OccurredAt: time.Now().UTC()}
	var group sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			errs <- consumer.HandleStatus(ctx, event)
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	// Callback receive time differs on replay; identity must not depend on it.
	event.OccurredAt = event.OccurredAt.Add(time.Hour)
	require.NoError(t, consumer.HandleStatus(ctx, event))
	event.Status = "delivered"
	require.NoError(t, consumer.HandleStatus(ctx, event))
	keyword := sms.InboundKeyword{ProviderMessageID: "SMinbound", FromE164: "+15555550123", ToE164: "+15555550456", Keyword: "STOP", ReceivedAt: time.Now().UTC()}
	require.NoError(t, consumer.HandleKeyword(ctx, keyword))
	keyword.ReceivedAt = keyword.ReceivedAt.Add(time.Hour)
	require.NoError(t, consumer.HandleKeyword(ctx, keyword))
	var count int
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.sms_callback_events`).Scan(&count))
	require.Equal(t, 3, count)
	var phone, storedKeyword string
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT payload->>'from', payload->>'keyword' FROM trakrf.sms_callback_events WHERE event_type='keyword'`).Scan(&phone, &storedKeyword))
	require.Equal(t, keyword.FromE164, phone)
	require.Equal(t, "STOP", storedKeyword)
	// Another configured account/service has an independent namespace.
	require.NoError(t, db.Store.SMSCallbackConsumer("ACother", "MGother").HandleStatus(ctx, event))
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.sms_callback_events`).Scan(&count))
	require.Equal(t, 4, count)
	// A fresh consumer sees the same durable deduplication state.
	require.NoError(t, db.Store.SMSCallbackConsumer("ACtest", "MGtest").HandleStatus(ctx, event))
	require.NoError(t, db.AdminPool.QueryRow(ctx, `SELECT count(*) FROM trakrf.sms_callback_events`).Scan(&count))
	require.Equal(t, 4, count)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	require.Error(t, consumer.HandleStatus(canceled, event))
}
