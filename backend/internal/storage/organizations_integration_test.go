//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestGetOrganizationByID_EntitlementFields(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()

	org, err := store.CreateOrganization(ctx, "Entitlement Co", "entitlement-co")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	got, err := store.GetOrganizationByID(ctx, org.ID)
	if err != nil {
		t.Fatalf("get org: %v", err)
	}
	if !got.SubscriptionEnabled {
		t.Errorf("SubscriptionEnabled = false, want true")
	}
	if got.SubscriptionExpiresAt != nil {
		t.Errorf("SubscriptionExpiresAt = %v, want nil", got.SubscriptionExpiresAt)
	}
}

func TestOrgIsEntitled_TruthTable(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)

	org, err := store.CreateOrganization(ctx, "Gate Co", "gate-co")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	cases := []struct {
		name    string
		enabled bool
		expires string // SQL expression for subscription_expires_at
		want    bool
	}{
		{"enabled, no expiry", true, "NULL", true},
		{"enabled, future expiry", true, "now() + interval '1 day'", true},
		{"enabled, past expiry (lapsed)", true, "now() - interval '1 day'", false},
		{"disabled", false, "NULL", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := pool.Exec(ctx,
				"UPDATE trakrf.organizations SET subscription_enabled=$1, subscription_expires_at="+c.expires+" WHERE id=$2",
				c.enabled, org.ID)
			if err != nil {
				t.Fatalf("update fixture: %v", err)
			}
			got, err := store.OrgIsEntitled(ctx, org.ID)
			if err != nil {
				t.Fatalf("OrgIsEntitled: %v", err)
			}
			if got != c.want {
				t.Errorf("OrgIsEntitled = %v, want %v", got, c.want)
			}
		})
	}
}
