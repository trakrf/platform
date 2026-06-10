//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// seedOrgMember inserts a user and an active org_users membership row.
func seedOrgMember(t *testing.T, pool *pgxpool.Pool, orgID int, email string) int {
	t.Helper()
	ctx := context.Background()
	var userID int
	err := pool.QueryRow(ctx,
		`INSERT INTO trakrf.users (name, email, password_hash) VALUES ($1, $2, 'stub') RETURNING id`,
		email, email).Scan(&userID)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO trakrf.org_users (org_id, user_id, role, status) VALUES ($1, $2, 'admin', 'active')`,
		orgID, userID)
	if err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return userID
}

// TRA-949: ListAllOrgs returns every non-deleted org (regardless of membership),
// with the entitlement fields and an accurate member count, ordered by name.
func TestListAllOrgs_ReturnsAllWithMemberCounts(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)

	// "Alpha" has two members; "Beta" has none.
	alpha, err := store.CreateOrganization(ctx, "Alpha", "alpha-org")
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	beta, err := store.CreateOrganization(ctx, "Beta", "beta-org")
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}
	seedOrgMember(t, pool, alpha.ID, "a1@example.com")
	seedOrgMember(t, pool, alpha.ID, "a2@example.com")

	// Beta is lapsed (disabled) to confirm entitlement fields surface raw.
	_, err = pool.Exec(ctx,
		`UPDATE trakrf.organizations SET subscription_enabled = false WHERE id = $1`, beta.ID)
	if err != nil {
		t.Fatalf("disable beta: %v", err)
	}

	orgs, err := store.ListAllOrgs(ctx)
	if err != nil {
		t.Fatalf("ListAllOrgs: %v", err)
	}
	if len(orgs) != 2 {
		t.Fatalf("len = %d, want 2 (%+v)", len(orgs), orgs)
	}
	// Ordered by name ascending.
	if orgs[0].Name != "Alpha" || orgs[1].Name != "Beta" {
		t.Fatalf("order = [%s, %s], want [Alpha, Beta]", orgs[0].Name, orgs[1].Name)
	}
	if orgs[0].MemberCount != 2 {
		t.Errorf("Alpha MemberCount = %d, want 2", orgs[0].MemberCount)
	}
	if orgs[1].MemberCount != 0 {
		t.Errorf("Beta MemberCount = %d, want 0", orgs[1].MemberCount)
	}
	if !orgs[0].SubscriptionEnabled {
		t.Errorf("Alpha SubscriptionEnabled = false, want true")
	}
	if orgs[1].SubscriptionEnabled {
		t.Errorf("Beta SubscriptionEnabled = true, want false")
	}
}

// Soft-deleted orgs must not appear in the superadmin all-orgs list.
func TestListAllOrgs_ExcludesDeleted(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)

	keep, err := store.CreateOrganization(ctx, "Keep", "keep-org")
	if err != nil {
		t.Fatalf("create keep: %v", err)
	}
	gone, err := store.CreateOrganization(ctx, "Gone", "gone-org")
	if err != nil {
		t.Fatalf("create gone: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE trakrf.organizations SET deleted_at = now() WHERE id = $1`, gone.ID); err != nil {
		t.Fatalf("soft-delete gone: %v", err)
	}

	orgs, err := store.ListAllOrgs(ctx)
	if err != nil {
		t.Fatalf("ListAllOrgs: %v", err)
	}
	if len(orgs) != 1 || orgs[0].ID != keep.ID {
		t.Fatalf("got %+v, want only Keep (id=%d)", orgs, keep.ID)
	}
}

// TRA-949: UpdateOrgEntitlement sets the kill switch + expiry and the change is
// immediately reflected by org_is_entitled.
func TestUpdateOrgEntitlement_PersistsAndAffectsEntitlement(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()

	org, err := store.CreateOrganization(ctx, "Toggle Co", "toggle-co")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Disable entitlement entirely.
	updated, err := store.UpdateOrgEntitlement(ctx, org.ID, false, nil)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if updated.SubscriptionEnabled {
		t.Errorf("SubscriptionEnabled = true, want false")
	}
	entitled, err := store.OrgIsEntitled(ctx, org.ID)
	if err != nil {
		t.Fatalf("OrgIsEntitled: %v", err)
	}
	if entitled {
		t.Errorf("entitled = true after disable, want false")
	}

	// Re-enable with a future expiry.
	future := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	updated, err = store.UpdateOrgEntitlement(ctx, org.ID, true, &future)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !updated.SubscriptionEnabled {
		t.Errorf("SubscriptionEnabled = false, want true")
	}
	if updated.SubscriptionExpiresAt == nil || !updated.SubscriptionExpiresAt.Equal(future) {
		t.Errorf("SubscriptionExpiresAt = %v, want %v", updated.SubscriptionExpiresAt, future)
	}
	entitled, err = store.OrgIsEntitled(ctx, org.ID)
	if err != nil {
		t.Fatalf("OrgIsEntitled: %v", err)
	}
	if !entitled {
		t.Errorf("entitled = false after enable+future, want true")
	}

	// Clearing the expiry (NULL = never expires) keeps it entitled.
	updated, err = store.UpdateOrgEntitlement(ctx, org.ID, true, nil)
	if err != nil {
		t.Fatalf("clear expiry: %v", err)
	}
	if updated.SubscriptionExpiresAt != nil {
		t.Errorf("SubscriptionExpiresAt = %v, want nil after clear", updated.SubscriptionExpiresAt)
	}
}

// A past expiry lapses the org even though the kill switch is on.
func TestUpdateOrgEntitlement_PastExpiryLapses(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()

	org, err := store.CreateOrganization(ctx, "Lapse Co", "lapse-co")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	past := time.Now().Add(-1 * time.Hour)
	if _, err := store.UpdateOrgEntitlement(ctx, org.ID, true, &past); err != nil {
		t.Fatalf("set past expiry: %v", err)
	}
	entitled, err := store.OrgIsEntitled(ctx, org.ID)
	if err != nil {
		t.Fatalf("OrgIsEntitled: %v", err)
	}
	if entitled {
		t.Errorf("entitled = true with past expiry, want false")
	}
}

// Updating a non-existent org returns (nil, nil) per the no-rows convention.
func TestUpdateOrgEntitlement_MissingOrg(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()

	got, err := store.UpdateOrgEntitlement(ctx, 999999, true, nil)
	if err != nil {
		t.Fatalf("UpdateOrgEntitlement missing: unexpected err %v", err)
	}
	if got != nil {
		t.Errorf("got = %+v, want nil for missing org", got)
	}
}
