//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/notificationdelivery"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
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

func TestNotificationDelivery_InsertIfAbsentRoundTripsRoutingContext(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-ND-ROUTE")
	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{
		Name: "Priya", Email: strPtr("priya@acme.test"),
	})
	require.NoError(t, err)

	eventID := "evt-route-1"
	payload := []byte(`{"to":"priya@acme.test","subject":"Asset moved","body":"hi"}`)
	d := notificationdelivery.NotificationDelivery{
		DeliveryID:  "evt-route-1:recipient:email",
		Channel:     notificationdelivery.ChannelEmail,
		State:       notificationdelivery.StatePending,
		EventID:     &eventID,
		RecipientID: &r.ID,
		AssetID:     &a.ID,
		Payload:     payload,
	}

	var firstID int64
	var inserted bool
	err = db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		firstID, inserted, err = db.Store.InsertNotificationDeliveryIfAbsentTx(ctx, tx, orgID, d)
		return err
	})
	require.NoError(t, err)
	require.True(t, inserted)
	require.NotZero(t, firstID)

	got, err := db.Store.GetNotificationDeliveryByDeliveryID(ctx, orgID, d.DeliveryID)
	require.NoError(t, err)
	require.Equal(t, firstID, got.ID)
	require.Equal(t, eventID, *got.EventID)
	require.Equal(t, r.ID, *got.RecipientID)
	require.Equal(t, a.ID, *got.AssetID)
	require.JSONEq(t, string(payload), string(got.Payload))

	// A second insert of the same delivery is a no-op, not a duplicate or an error.
	other := d
	otherEvent := "evt-route-other"
	other.EventID = &otherEvent
	other.Payload = []byte(`{"body":"changed"}`)
	var secondID int64
	err = db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		secondID, inserted, err = db.Store.InsertNotificationDeliveryIfAbsentTx(ctx, tx, orgID, other)
		return err
	})
	require.NoError(t, err)
	require.False(t, inserted)
	require.Zero(t, secondID)

	got, err = db.Store.GetNotificationDeliveryByDeliveryID(ctx, orgID, d.DeliveryID)
	require.NoError(t, err)
	require.Equal(t, firstID, got.ID)
	require.Equal(t, eventID, *got.EventID)
	require.JSONEq(t, string(payload), string(got.Payload))

	// The worker's lookup by River job ID reads the same columns.
	err = db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return db.Store.SetNotificationDeliveryRiverJobIDTx(ctx, tx, orgID, firstID, 424242)
	})
	require.NoError(t, err)
	byJob, err := db.Store.GetNotificationDeliveryByRiverJobID(ctx, orgID, 424242)
	require.NoError(t, err)
	require.Equal(t, eventID, *byJob.EventID)
	require.Equal(t, r.ID, *byJob.RecipientID)
	require.Equal(t, a.ID, *byJob.AssetID)
	require.JSONEq(t, string(payload), string(byJob.Payload))
}

func TestNotificationDelivery_InsertIfAbsentNilRoutingContextReadsBackNil(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	var inserted bool
	err := db.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		_, inserted, err = db.Store.InsertNotificationDeliveryIfAbsentTx(ctx, tx, orgID, notificationdelivery.NotificationDelivery{
			DeliveryID: "evt-bare:recipient:sms",
			Channel:    notificationdelivery.ChannelSMS,
			State:      notificationdelivery.StatePending,
			Payload:    []byte{},
		})
		return err
	})
	require.NoError(t, err)
	require.True(t, inserted)

	got, err := db.Store.GetNotificationDeliveryByDeliveryID(ctx, orgID, "evt-bare:recipient:sms")
	require.NoError(t, err)
	require.Nil(t, got.EventID)
	require.Nil(t, got.RecipientID)
	require.Nil(t, got.AssetID)
	require.Nil(t, got.Payload)
}
