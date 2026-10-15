//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// TRA-1047: each org with a manual expiry gets exactly one notice per stage
// (T-14, T-3, at expiry, at cutoff), and only the stage it is currently in.
func TestListDueSubscriptionNotices_Stages(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)

	mkOrg := func(ident, expires string, enabled bool) int {
		t.Helper()
		org, err := store.CreateOrganization(ctx, ident, ident)
		require.NoError(t, err)
		_, err = pool.Exec(ctx,
			`UPDATE trakrf.organizations SET subscription_enabled=$1, subscription_expires_at=`+expires+` WHERE id=$2`,
			enabled, org.ID)
		require.NoError(t, err)
		return org.ID
	}

	far := mkOrg("far-co", "now() + interval '30 days'", true)
	t14 := mkOrg("t14-co", "now() + interval '10 days'", true)
	t3 := mkOrg("t3-co", "now() + interval '2 days'", true)
	expired := mkOrg("expired-co", "now() - interval '1 day'", true)
	cutoff := mkOrg("cutoff-co", "now() - interval '4 days'", true)
	stale := mkOrg("stale-co", "now() - interval '60 days'", true)
	perpetual := mkOrg("perpetual-co", "NULL", true)
	disabled := mkOrg("disabled-co", "now() + interval '2 days'", false)

	due, err := store.ListDueSubscriptionNotices(ctx)
	require.NoError(t, err)

	got := map[int]storage.SubscriptionNoticeKind{}
	for _, n := range due {
		got[n.OrgID] = n.Kind
		assert.True(t, n.CutoffAt.After(n.ExpiresAt), "cutoff must include grace")
	}
	assert.Equal(t, storage.NoticeTMinus14, got[t14])
	assert.Equal(t, storage.NoticeTMinus3, got[t3])
	assert.Equal(t, storage.NoticeExpired, got[expired])
	assert.Equal(t, storage.NoticeCutoff, got[cutoff])
	for name, id := range map[string]int{"far": far, "stale": stale, "perpetual": perpetual, "disabled": disabled} {
		_, listed := got[id]
		assert.False(t, listed, "%s org must not be due", name)
	}

	// Claiming is send-once per (org, kind, expiry).
	var n storage.SubscriptionNotice
	for _, d := range due {
		if d.OrgID == t3 {
			n = d
		}
	}
	claimed, err := store.ClaimSubscriptionNotice(ctx, n.OrgID, n.Kind, n.ExpiresAt)
	require.NoError(t, err)
	assert.True(t, claimed)
	claimed, err = store.ClaimSubscriptionNotice(ctx, n.OrgID, n.Kind, n.ExpiresAt)
	require.NoError(t, err)
	assert.False(t, claimed, "second claim must lose")

	due, err = store.ListDueSubscriptionNotices(ctx)
	require.NoError(t, err)
	for _, d := range due {
		assert.NotEqual(t, t3, d.OrgID, "claimed notice must not be due again")
	}

	// A renewal (new expiry) starts a fresh sequence.
	_, err = pool.Exec(ctx,
		`UPDATE trakrf.organizations SET subscription_expires_at = now() + interval '1 day' WHERE id=$1`, t3)
	require.NoError(t, err)
	due, err = store.ListDueSubscriptionNotices(ctx)
	require.NoError(t, err)
	found := false
	for _, d := range due {
		found = found || d.OrgID == t3
	}
	assert.True(t, found, "a new expiry must be due again")
}

// Orgs paying through an active subscription row are not nagged about a stale
// manual expiry.
func TestListDueSubscriptionNotices_SkipsActiveSubscription(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)

	org, err := store.CreateOrganization(ctx, "Paid Co", "paid-co")
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`UPDATE trakrf.organizations SET subscription_expires_at = now() + interval '2 days' WHERE id=$1`, org.ID)
	require.NoError(t, err)
	var planID int64
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO trakrf.subscription_plans (name) VALUES ('Plan') RETURNING id`).Scan(&planID))
	_, err = pool.Exec(ctx,
		`INSERT INTO trakrf.subscriptions (org_id, plan_id, status, current_period_end)
		 VALUES ($1, $2, 'active', now() + interval '30 days')`, org.ID, planID)
	require.NoError(t, err)

	due, err := store.ListDueSubscriptionNotices(ctx)
	require.NoError(t, err)
	for _, d := range due {
		assert.NotEqual(t, org.ID, d.OrgID)
	}
}

// Nag recipients are the org's admins only.
func TestListOrgAdminEmails(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)

	org, err := store.CreateOrganization(ctx, "Admins Co", "admins-co")
	require.NoError(t, err)
	b := seedUser(t, pool, "b-admin@example.com", false)
	a := seedUser(t, pool, "a-admin@example.com", false)
	v := seedUser(t, pool, "viewer@example.com", false)
	gone := seedUser(t, pool, "gone-admin@example.com", false)
	require.NoError(t, store.AddUserToOrg(ctx, org.ID, b, models.RoleAdmin))
	require.NoError(t, store.AddUserToOrg(ctx, org.ID, a, models.RoleAdmin))
	require.NoError(t, store.AddUserToOrg(ctx, org.ID, v, models.RoleViewer))
	require.NoError(t, store.AddUserToOrg(ctx, org.ID, gone, models.RoleAdmin))
	_, err = pool.Exec(ctx, `UPDATE trakrf.users SET deleted_at = now() WHERE id=$1`, gone)
	require.NoError(t, err)

	emails, err := store.ListOrgAdminEmails(ctx, org.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"a-admin@example.com", "b-admin@example.com"}, emails)
}
