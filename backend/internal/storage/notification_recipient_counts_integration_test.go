//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/notificationrecipient"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// requireSubscribedAssetCount checks the count on both read paths, so the list
// and single-contact queries can't drift apart.
func requireSubscribedAssetCount(t *testing.T, db *testutil.TestDB, orgID, recipientID, want int) {
	t.Helper()
	ctx := context.Background()

	got, err := db.Store.GetNotificationRecipient(ctx, orgID, recipientID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, want, got.SubscribedAssetCount, "single-contact count")

	list, err := db.Store.ListNotificationRecipients(ctx, orgID)
	require.NoError(t, err)
	for _, r := range list {
		if r.ID == recipientID {
			require.Equal(t, want, r.SubscribedAssetCount, "list count")
			return
		}
	}
	require.Failf(t, "contact missing from list", "recipient %d", recipientID)
}

func TestNotificationRecipients_SubscribedAssetCount_CountsDistinctAssets(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	assetA := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NC-A")
	assetB := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NC-B")
	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{
		Name: "Both channels", Email: strPtr("both@acme.test"), Phone: strPtr("+15550200"),
	})
	require.NoError(t, err)
	requireSubscribedAssetCount(t, db, orgID, r.ID, 0)

	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, assetA.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, assetA.ID, r.ID, notificationrecipient.ChannelSMS)
	require.NoError(t, err)
	requireSubscribedAssetCount(t, db, orgID, r.ID, 1)

	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, assetB.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	requireSubscribedAssetCount(t, db, orgID, r.ID, 2)
}

func TestNotificationRecipients_SubscribedAssetCount_IgnoresSwitchedOff(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NC-OFF")
	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "Off", Email: strPtr("off@acme.test")})
	require.NoError(t, err)
	sub, _, err := db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	requireSubscribedAssetCount(t, db, orgID, r.ID, 1)

	_, err = db.Store.UpdateAssetNotificationSubscription(ctx, orgID, a.ID, sub.ID, notificationrecipient.UpdateSubscriptionRequest{IsActive: boolPtr(false)})
	require.NoError(t, err)
	requireSubscribedAssetCount(t, db, orgID, r.ID, 0)

	_, err = db.Store.UpdateAssetNotificationSubscription(ctx, orgID, a.ID, sub.ID, notificationrecipient.UpdateSubscriptionRequest{IsActive: boolPtr(true)})
	require.NoError(t, err)
	requireSubscribedAssetCount(t, db, orgID, r.ID, 1)
}

func TestNotificationRecipients_SubscribedAssetCount_IgnoresDeletedAssets(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NC-DEL")
	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "Del", Email: strPtr("del@acme.test")})
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	requireSubscribedAssetCount(t, db, orgID, r.ID, 1)

	deleted, err := db.Store.DeleteAsset(ctx, orgID, a.ID)
	require.NoError(t, err)
	require.True(t, deleted)
	requireSubscribedAssetCount(t, db, orgID, r.ID, 0)
}

func TestNotificationRecipients_SubscribedAssetCount_IncludesPausedContact(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	a := testutil.CreateTestAsset(t, db.AdminPool, orgID, "ASSET-NC-PAUSE")
	r, err := db.Store.CreateNotificationRecipient(ctx, orgID, notificationrecipient.CreateRecipientRequest{Name: "Paused", Email: strPtr("paused@acme.test")})
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgID, a.ID, r.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)

	_, err = db.Store.UpdateNotificationRecipient(ctx, orgID, r.ID, notificationrecipient.UpdateRecipientRequest{IsActive: boolPtr(false)})
	require.NoError(t, err)
	requireSubscribedAssetCount(t, db, orgID, r.ID, 1)
}

func TestNotificationRecipients_SubscribedAssetCount_IsolatedByOrg(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	orgB := createOrg(t, db.AdminPool, "Org B Contact Count", "test-org-b-contact-count")
	assetA := testutil.CreateTestAsset(t, db.AdminPool, orgA, "ASSET-NC-ORGA")
	assetB := testutil.CreateTestAsset(t, db.AdminPool, orgB, "ASSET-NC-ORGB")
	// The same email in both orgs is allowed; only the org's own row may count.
	inA, err := db.Store.CreateNotificationRecipient(ctx, orgA, notificationrecipient.CreateRecipientRequest{Name: "A", Email: strPtr("same@acme.test")})
	require.NoError(t, err)
	inB, err := db.Store.CreateNotificationRecipient(ctx, orgB, notificationrecipient.CreateRecipientRequest{Name: "B", Email: strPtr("same@acme.test")})
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgA, assetA.ID, inA.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)
	_, _, err = db.Store.CreateAssetNotificationSubscription(ctx, orgB, assetB.ID, inB.ID, notificationrecipient.ChannelEmail)
	require.NoError(t, err)

	requireSubscribedAssetCount(t, db, orgA, inA.ID, 1)
	requireSubscribedAssetCount(t, db, orgB, inB.ID, 1)

	crossOrg, err := db.Store.GetNotificationRecipient(ctx, orgA, inB.ID)
	require.NoError(t, err)
	require.Nil(t, crossOrg, "another org's contact must not be readable")
}
