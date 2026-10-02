//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestNotificationDelivery_InsertGetUpdate(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	var insertedID int64
	err := db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		id, err := db.Store.InsertNotificationDeliveryTx(ctx, tx, orgID, notificationdelivery.NotificationDelivery{
			DeliveryID: "evt-1:contact-1:email",
			Channel:    notificationdelivery.ChannelEmail,
			State:      notificationdelivery.StatePending,
		})
		insertedID = id
		return err
	})
	require.NoError(t, err)
	require.NotZero(t, insertedID)

	got, err := db.Store.GetNotificationDeliveryByDeliveryID(ctx, orgID, "evt-1:contact-1:email")
	require.NoError(t, err)
	require.Equal(t, notificationdelivery.StatePending, got.State)
	require.Equal(t, 0, got.AttemptCount)

	providerMsgID := "resend-msg-123"
	err = db.Store.UpdateNotificationDeliveryState(ctx, orgID, insertedID, notificationdelivery.StateDelivered, nil, &providerMsgID)
	require.NoError(t, err)

	got, err = db.Store.GetNotificationDeliveryByDeliveryID(ctx, orgID, "evt-1:contact-1:email")
	require.NoError(t, err)
	require.Equal(t, notificationdelivery.StateDelivered, got.State)
	require.Equal(t, 1, got.AttemptCount)
	require.Equal(t, "resend-msg-123", *got.ProviderMessageID)
	require.NotNil(t, got.FinalizedAt)

	// Cross-org isolation: a different org must not see this row.
	// testutil.CreateTestAccount hardcodes identifier="test-org" (unique per
	// test DB), so a second org for cross-tenant checks goes through the
	// package's createOrg helper (see cross_org_helper_test.go) instead of a
	// second CreateTestAccount call, which would collide on that constraint.
	otherOrgID := createOrg(t, db.AdminPool, "Notification Delivery Org B", "notification-delivery-org-b")
	_, err = db.Store.GetNotificationDeliveryByDeliveryID(ctx, otherOrgID, "evt-1:contact-1:email")
	require.Error(t, err)
}
