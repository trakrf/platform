//go:build integration

package storage_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func strPtr(s string) *string { return &s }

func TestNotificationSubscriptions_ZeroSubscribersIsValid(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NS-ZERO")

	subs, err := db.Store.ListAssetNotificationSubscriptions(ctx, orgID, a.ID)
	require.NoError(t, err)
	require.Empty(t, subs)
}

func TestNotificationSubscriptions_SeveralSubscribersPerAsset(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NS-MANY")

	r1, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "Priya", Email: strPtr("priya@acme.test")})
	require.NoError(t, err)
	r2, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "Sam", Phone: strPtr("+15550100")})
	require.NoError(t, err)

	_, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r1.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	_, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r2.ID, notificationrecipient.ChannelSMS)
	require.NoError(t, err)

	subs, err := db.Store.ListAssetNotificationSubscriptions(ctx, orgID, a.ID)
	require.NoError(t, err)
	require.Len(t, subs, 2)
}

func TestNotificationSubscriptions_ChannelMustMatchContact(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NS-CHAN")

	emailOnly, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "E", Email: strPtr("e@acme.test")})
	require.NoError(t, err)
	phoneOnly, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "P", Phone: strPtr("+15550101")})
	require.NoError(t, err)

	_, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, emailOnly.ID, notificationrecipient.ChannelSMS)
	require.ErrorIs(t, err, storage.ErrChannelContactMismatch)

	_, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, phoneOnly.ID, notificationrecipient.ChannelEmail)
	require.ErrorIs(t, err, storage.ErrChannelContactMismatch)
}

func TestNotificationSubscriptions_TriggerRejectsMismatchedChannel(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NS-TRIG")

	var recipientID int
	require.NoError(t, db.AdminPool.QueryRow(ctx,
		`INSERT INTO trakrf.notification_recipients (org_id, name, phone) VALUES ($1, 'T', '+15550102') RETURNING id`,
		orgID).Scan(&recipientID))

	_, err := db.AdminPool.Exec(ctx,
		`INSERT INTO trakrf.asset_notification_recipients (org_id, asset_id, recipient_id, channel) VALUES ($1, $2, $3, 'email')`,
		orgID, a.ID, recipientID)
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr))
	require.Equal(t, "23514", pgErr.Code)
}

func TestNotificationRecipients_DuplicateContactIsConflict(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	_, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "A", Email: strPtr("dup@acme.test")})
	require.NoError(t, err)

	_, err = db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "B", Email: strPtr("DUP@acme.test")})
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr))
	require.Equal(t, "23505", pgErr.Code)
}

func TestNotificationRecipients_DeletedContactDoesNotBlockReAdd(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	first, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "A", Email: strPtr("back@acme.test")})
	require.NoError(t, err)
	ok, err := db.Store.DeleteNotificationRecipient(ctx, orgID, first.ID)
	require.NoError(t, err)
	require.True(t, ok)

	_, err = db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "A again", Email: strPtr("back@acme.test")})
	require.NoError(t, err)
}

func TestNotificationRecipients_DeleteRemovesSubscriptions(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NS-DEL")

	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "D", Email: strPtr("d@acme.test")})
	require.NoError(t, err)
	_, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)

	_, err = db.Store.DeleteNotificationRecipient(ctx, orgID, r.ID)
	require.NoError(t, err)

	subs, err := db.Store.ListAssetNotificationSubscriptions(ctx, orgID, a.ID)
	require.NoError(t, err)
	require.Empty(t, subs)
}

func TestNotificationSubscriptions_CrossOrgIsolation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	orgB := createOrg(t, db.AdminPool, "Org B Notification", "test-org-b-notification")
	assetA := testutil.CreateTestAsset(t, db.AdminPool, orgA, "ASSET-NS-XORG")

	recipientA, err := db.Store.CreateNotificationRecipient(ctx, orgA, notificationrecipient.CreateRecipientRequest{Name: "A", Email: strPtr("a@acme.test")})
	require.NoError(t, err)

	got, err := db.Store.GetNotificationRecipient(ctx, orgB, recipientA.ID)
	require.NoError(t, err)
	require.Nil(t, got)

	list, err := db.Store.ListNotificationRecipients(ctx, orgB)
	require.NoError(t, err)
	require.Empty(t, list)

	_, err = db.Store.CreateAssetNotificationSubscription(ctx, orgB, assetA.ID, recipientA.ID, notificationrecipient.ChannelEmail)
	require.ErrorIs(t, err, storage.ErrSubscriptionAssetNotFound)

	orgBRecipient, err := db.Store.CreateNotificationRecipient(ctx, orgB, notificationrecipient.CreateRecipientRequest{Name: "B", Email: strPtr("b@acme.test")})
	require.NoError(t, err)
	_, err = db.Store.CreateAssetNotificationSubscription(ctx, orgB, assetA.ID, orgBRecipient.ID, notificationrecipient.ChannelEmail)
	require.ErrorIs(t, err, storage.ErrSubscriptionAssetNotFound)
}
