//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/webhook"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestWebhook_CRUD(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	created, err := db.Store.CreateWebhook(ctx, orgID, "https://example.com/hook", "whsec_abc", true)
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, orgID, created.OrgID)
	require.Equal(t, "https://example.com/hook", created.URL)
	require.Equal(t, "whsec_abc", created.Secret)
	require.True(t, created.Enabled)

	got, err := db.Store.GetWebhook(ctx, orgID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, created.ID, got.ID)

	byID, err := db.Store.GetWebhookByID(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.NotNil(t, byID)
	require.Equal(t, created.ID, byID.ID)

	missing, err := db.Store.GetWebhookByID(ctx, orgID, 99999999)
	require.NoError(t, err)
	require.Nil(t, missing)

	// Partial update: url only, enabled untouched.
	newURL := "https://example.com/hook-2"
	updated, err := db.Store.UpdateWebhook(ctx, orgID, created.ID, webhook.UpdateRequest{URL: &newURL})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, newURL, updated.URL)
	require.True(t, updated.Enabled)

	// Partial update: enabled only, url untouched.
	off := false
	updated, err = db.Store.UpdateWebhook(ctx, orgID, created.ID, webhook.UpdateRequest{Enabled: &off})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.Equal(t, newURL, updated.URL)
	require.False(t, updated.Enabled)

	deleted, err := db.Store.DeleteWebhook(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	gone, err := db.Store.GetWebhook(ctx, orgID)
	require.NoError(t, err)
	require.Nil(t, gone)

	// Deleting twice reports no row affected rather than erroring.
	deleted, err = db.Store.DeleteWebhook(ctx, orgID, created.ID)
	require.NoError(t, err)
	require.False(t, deleted)
}

func TestWebhook_OnePerOrg(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	first, err := db.Store.CreateWebhook(ctx, orgID, "https://example.com/a", "whsec_a", true)
	require.NoError(t, err)

	_, err = db.Store.CreateWebhook(ctx, orgID, "https://example.com/b", "whsec_b", true)
	require.ErrorIs(t, err, storage.ErrWebhookExists)

	// The unique index is partial on deleted_at IS NULL, so re-registering
	// after a delete must succeed.
	_, err = db.Store.DeleteWebhook(ctx, orgID, first.ID)
	require.NoError(t, err)

	second, err := db.Store.CreateWebhook(ctx, orgID, "https://example.com/b", "whsec_b", true)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
}

func TestWebhook_OrgIsolation(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	orgB := createOrg(t, db.AdminPool, "Webhook Org B", "webhook-org-b")

	created, err := db.Store.CreateWebhook(ctx, orgA, "https://a.example.com/hook", "whsec_a", true)
	require.NoError(t, err)

	// Org B sees nothing of org A's, by id or by org.
	got, err := db.Store.GetWebhook(ctx, orgB)
	require.NoError(t, err)
	require.Nil(t, got)

	byID, err := db.Store.GetWebhookByID(ctx, orgB, created.ID)
	require.NoError(t, err)
	require.Nil(t, byID)

	// ...and cannot mutate it either.
	newURL := "https://attacker.example.com/hook"
	updated, err := db.Store.UpdateWebhook(ctx, orgB, created.ID, webhook.UpdateRequest{URL: &newURL})
	require.NoError(t, err)
	require.Nil(t, updated)

	deleted, err := db.Store.DeleteWebhook(ctx, orgB, created.ID)
	require.NoError(t, err)
	require.False(t, deleted)

	still, err := db.Store.GetWebhook(ctx, orgA)
	require.NoError(t, err)
	require.NotNil(t, still)
	require.Equal(t, "https://a.example.com/hook", still.URL)
}

func TestWebhook_GetForDelivery(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, db.AdminPool)

	// No webhook yet.
	wh, _, err := db.Store.GetWebhookForDelivery(ctx, orgID)
	require.NoError(t, err)
	require.Nil(t, wh)

	created, err := db.Store.CreateWebhook(ctx, orgID, "https://example.com/hook", "whsec_abc", true)
	require.NoError(t, err)

	wh, entitled, err := db.Store.GetWebhookForDelivery(ctx, orgID)
	require.NoError(t, err)
	require.NotNil(t, wh)
	require.Equal(t, created.ID, wh.ID)
	require.Equal(t, "whsec_abc", wh.Secret, "the delivery path needs the cleartext secret to sign")
	require.True(t, wh.Enabled)
	require.True(t, entitled, "a fresh test org is entitled by default")

	// An abandoned trial: the row survives, entitlement does not. The caller
	// must be able to tell this apart from "no webhook" and from "disabled".
	_, err = db.AdminPool.Exec(ctx,
		`UPDATE trakrf.organizations SET subscription_enabled = false WHERE id = $1`, orgID)
	require.NoError(t, err)

	wh, entitled, err = db.Store.GetWebhookForDelivery(ctx, orgID)
	require.NoError(t, err)
	require.NotNil(t, wh)
	require.True(t, wh.Enabled)
	require.False(t, entitled)
}
