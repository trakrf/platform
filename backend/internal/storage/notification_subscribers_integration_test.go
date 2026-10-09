//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestSubscribersForAsset_NoneIsEmptyNotNil(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-SUB-NONE")

	subs, err := db.Store.ListSubscribersForAsset(ctx, orgID, a.ID)
	require.NoError(t, err)
	require.NotNil(t, subs)
	require.Empty(t, subs)
}

func TestSubscribersForAsset_OneRowPerChannel(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-SUB-BOTH")
	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "Both", Email: strPtr("both@acme.test"), Phone: strPtr("+15550200")})
	require.NoError(t, err)

	emailSub, _, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	smsSub, _, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelSMS)
	require.NoError(t, err)

	subs, err := db.Store.ListSubscribersForAsset(ctx, orgID, a.ID)
	require.NoError(t, err)
	require.Len(t, subs, 2)
	// Ids are obfuscated, not sequential: the order is by id, not by creation.
	require.Less(t, subs[0].SubscriptionID, subs[1].SubscriptionID)
	byChannel := map[string]int{}
	for _, s := range subs {
		byChannel[s.Channel] = s.SubscriptionID
	}
	require.Equal(t, map[string]int{notificationrecipient.ChannelEmail: emailSub.ID, notificationrecipient.ChannelSMS: smsSub.ID}, byChannel)
	for _, s := range subs {
		require.Equal(t, r.ID, s.RecipientID)
		require.Equal(t, "Both", s.Name)
		require.Equal(t, r.Email, s.Email)
		require.Equal(t, r.Phone, s.Phone)
		require.True(t, s.RecipientActive)
		require.Nil(t, s.RecipientDeletedAt)
	}
}

func TestSubscribersForAsset_SwitchedOffIsAbsent(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-SUB-OFF")
	on, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "On", Email: strPtr("on@acme.test")})
	require.NoError(t, err)
	off, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "Off", Email: strPtr("off@acme.test")})
	require.NoError(t, err)

	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, on.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	offSub, _, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, off.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	_, err = db.Store.UpdateAssetNotificationSubscription(ctx, orgID, a.ID, offSub.ID, notificationrecipient.UpdateSubscriptionRequest{IsActive: boolPtr(false)})
	require.NoError(t, err)

	subs, err := db.Store.ListSubscribersForAsset(ctx, orgID, a.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, on.ID, subs[0].RecipientID)
}

func TestSubscribersForAsset_PausedRecipientIsReported(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-SUB-PAUSE")
	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "Paused", Email: strPtr("paused@acme.test")})
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	_, err = db.Store.UpdateNotificationRecipient(ctx, orgID, r.ID, notificationrecipient.UpdateRecipientRequest{IsActive: boolPtr(false)})
	require.NoError(t, err)

	subs, err := db.Store.ListSubscribersForAsset(ctx, orgID, a.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1, "a paused recipient is returned so routing can say why it was skipped")
	require.Equal(t, r.ID, subs[0].RecipientID)
	require.False(t, subs[0].RecipientActive)
}

func TestSubscribersForAsset_OtherOrgNeverReturned(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	orgB := createOrg(t, db.AdminPool, "Org B Subscribers", "test-org-b-subscribers")
	assetA := testutil.CreateTestAsset(t, db.AdminPool, orgA, "ASSET-SUB-XORG-A")
	assetB := testutil.CreateTestAsset(t, db.AdminPool, orgB, "ASSET-SUB-XORG-B")

	rA, err := db.Store.CreateNotificationRecipient(ctx, orgA, notificationrecipient.CreateRecipientRequest{Name: "A", Email: strPtr("xa@acme.test")})
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgA, assetA.ID, rA.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	rB, err := db.Store.CreateNotificationRecipient(ctx, orgB, notificationrecipient.CreateRecipientRequest{Name: "B", Email: strPtr("xb@acme.test")})
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgB, assetB.ID, rB.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)

	fromB, err := db.Store.ListSubscribersForAsset(ctx, orgB, assetA.ID)
	require.NoError(t, err)
	require.Empty(t, fromB, "org B must not see org A's subscribers")

	ownB, err := db.Store.ListSubscribersForAsset(ctx, orgB, assetB.ID)
	require.NoError(t, err)
	require.Len(t, ownB, 1)
	require.Equal(t, rB.ID, ownB[0].RecipientID)
}
