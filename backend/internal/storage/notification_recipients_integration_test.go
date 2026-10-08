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

	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r1.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r2.ID, notificationrecipient.ChannelSMS)
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

	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, emailOnly.ID, notificationrecipient.ChannelSMS)
	require.ErrorIs(t, err, storage.ErrChannelContactMismatch)

	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, phoneOnly.ID, notificationrecipient.ChannelEmail)
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

func TestNotificationRecipients_DeleteDeactivatesSubscriptions(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NS-DEL")

	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "D", Email: strPtr("d@acme.test")})
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)

	_, err = db.Store.DeleteNotificationRecipient(ctx, orgID, r.ID)
	require.NoError(t, err)

	subs, err := db.Store.ListAssetNotificationSubscriptions(ctx, orgID, a.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1, "the subscription is kept, not deleted")
	require.False(t, subs[0].IsActive)
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

	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgB, assetA.ID, recipientA.ID, notificationrecipient.ChannelEmail)
	require.ErrorIs(t, err, storage.ErrSubscriptionAssetNotFound)

	orgBRecipient, err := db.Store.CreateNotificationRecipient(ctx, orgB, notificationrecipient.CreateRecipientRequest{Name: "B", Email: strPtr("b@acme.test")})
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgB, assetA.ID, orgBRecipient.ID, notificationrecipient.ChannelEmail)
	require.ErrorIs(t, err, storage.ErrSubscriptionAssetNotFound)
}

func boolPtr(b bool) *bool { return &b }

func TestNotificationSubscriptions_ResubscribeReactivates(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NS-REACT")
	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "R", Email: strPtr("r@acme.test")})
	require.NoError(t, err)

	first, created, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	require.True(t, created)
	require.True(t, first.IsActive)

	again, created, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	require.False(t, created, "a repeat subscribe returns the existing subscription")
	require.Equal(t, first.ID, again.ID)
	require.Equal(t, first.UpdatedAt, again.UpdatedAt, "a repeat subscribe leaves an active row untouched")

	off, err := db.Store.UpdateAssetNotificationSubscription(ctx, orgID, a.ID, first.ID, notificationrecipient.UpdateSubscriptionRequest{IsActive: boolPtr(false)})
	require.NoError(t, err)
	require.False(t, off.IsActive)

	back, created, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, first.ID, back.ID)
	require.True(t, back.IsActive)
}

func TestNotificationSubscriptions_UpdateChannel(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NS-UPD")

	both, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "B", Email: strPtr("b@acme.test"), Phone: strPtr("+15550103")})
	require.NoError(t, err)
	emailOnly, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "E", Email: strPtr("e2@acme.test")})
	require.NoError(t, err)

	sub, _, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, both.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	got, err := db.Store.UpdateAssetNotificationSubscription(ctx, orgID, a.ID, sub.ID, notificationrecipient.UpdateSubscriptionRequest{Channel: strPtr(notificationrecipient.ChannelSMS)})
	require.NoError(t, err)
	require.Equal(t, notificationrecipient.ChannelSMS, got.Channel)

	sub2, _, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, emailOnly.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	_, err = db.Store.UpdateAssetNotificationSubscription(ctx, orgID, a.ID, sub2.ID, notificationrecipient.UpdateSubscriptionRequest{Channel: strPtr(notificationrecipient.ChannelSMS)})
	require.ErrorIs(t, err, storage.ErrChannelContactMismatch)

	missing, err := db.Store.UpdateAssetNotificationSubscription(ctx, orgID, a.ID, sub2.ID+1_000_000, notificationrecipient.UpdateSubscriptionRequest{IsActive: boolPtr(false)})
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestNotificationSubscriptions_ChannelChangeOntoExistingIsConflict(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NS-DUPCH")
	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "B", Email: strPtr("dc@acme.test"), Phone: strPtr("+15550104")})
	require.NoError(t, err)

	emailSub, _, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelSMS)
	require.NoError(t, err)

	_, err = db.Store.UpdateAssetNotificationSubscription(ctx, orgID, a.ID, emailSub.ID, notificationrecipient.UpdateSubscriptionRequest{Channel: strPtr(notificationrecipient.ChannelSMS)})
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr))
	require.Equal(t, "23505", pgErr.Code)
}

func TestNotificationSubscriptions_SwitchOffAfterAssetDeleted(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NS-GONE")
	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "G", Email: strPtr("g@acme.test")})
	require.NoError(t, err)
	sub, _, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)

	_, err = db.AdminPool.Exec(ctx, `UPDATE trakrf.assets SET deleted_at = NOW() WHERE id = $1`, a.ID)
	require.NoError(t, err)

	off, err := db.Store.UpdateAssetNotificationSubscription(ctx, orgID, a.ID, sub.ID, notificationrecipient.UpdateSubscriptionRequest{IsActive: boolPtr(false)})
	require.NoError(t, err)
	require.False(t, off.IsActive)

	_, err = db.Store.UpdateAssetNotificationSubscription(ctx, orgID, a.ID, sub.ID, notificationrecipient.UpdateSubscriptionRequest{IsActive: boolPtr(true)})
	require.ErrorIs(t, err, storage.ErrSubscriptionAssetNotFound)
}

func TestNotificationSubscriptions_TriggerRejectsRetargeting(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NS-RETGT")
	r1, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "1", Email: strPtr("rt1@acme.test")})
	require.NoError(t, err)
	r2, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "2", Email: strPtr("rt2@acme.test")})
	require.NoError(t, err)
	sub, _, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r1.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)

	_, err = db.AdminPool.Exec(ctx, `UPDATE trakrf.asset_notification_recipients SET recipient_id = $1 WHERE id = $2`, r2.ID, sub.ID)
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr))
	require.Equal(t, "23514", pgErr.Code)
}
