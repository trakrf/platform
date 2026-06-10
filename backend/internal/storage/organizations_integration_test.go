//go:build integration

package storage_test

import (
	"context"
	"testing"

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
