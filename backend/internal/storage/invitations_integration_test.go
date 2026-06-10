//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/trakrf/platform/backend/internal/models"
	"github.com/trakrf/platform/backend/internal/models/user"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// TestReInviteAfterCancel reproduces TRA-973: cancelling an invitation is a
// soft delete (sets cancelled_at, leaves the row), so re-inviting the same
// (org, email) must still succeed. The plain unique_org_email constraint used
// to collide with the lingering cancelled row, surfacing a 500.
func TestReInviteAfterCancel(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()

	org, err := store.CreateOrganization(ctx, "Organized Chaos", "organized-chaos")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	inviter, err := store.CreateUser(ctx, user.CreateUserRequest{
		Email:        "admin@example.com",
		Name:         "Admin",
		PasswordHash: "password-hash",
	})
	if err != nil {
		t.Fatalf("create inviter: %v", err)
	}

	const email = "tim.buckley@rfidready.net"
	expires := time.Now().Add(72 * time.Hour)

	// First invite.
	id1, err := store.CreateInvitation(ctx, org.ID, email, models.RoleViewer, "tokenhash-1", inviter.ID, expires)
	if err != nil {
		t.Fatalf("first invite: %v", err)
	}

	// Cancel it (soft delete — sets cancelled_at).
	if err := store.CancelInvitation(ctx, id1); err != nil {
		t.Fatalf("cancel invite: %v", err)
	}

	// Re-invite the same (org, email). This previously failed with a
	// unique_org_email violation against the lingering cancelled row.
	id2, err := store.CreateInvitation(ctx, org.ID, email, models.RoleViewer, "tokenhash-2", inviter.ID, expires)
	if err != nil {
		t.Fatalf("re-invite after cancel: %v", err)
	}
	if id2 == id1 {
		t.Errorf("re-invite returned same id %d as cancelled invite; expected a new row", id1)
	}

	// Exactly one live invite for (org, email).
	hasPending, err := store.HasPendingInvitation(ctx, org.ID, email)
	if err != nil {
		t.Fatalf("has pending: %v", err)
	}
	if !hasPending {
		t.Errorf("expected a live pending invitation after re-invite")
	}
}

// TestDuplicateLiveInviteRejected confirms the partial unique index still
// guards against two *live* invitations for the same (org, email) — the
// invariant the original constraint protected.
func TestDuplicateLiveInviteRejected(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()

	org, err := store.CreateOrganization(ctx, "Dup Co", "dup-co")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	inviter, err := store.CreateUser(ctx, user.CreateUserRequest{
		Email:        "admin@dup.example.com",
		Name:         "Admin",
		PasswordHash: "password-hash",
	})
	if err != nil {
		t.Fatalf("create inviter: %v", err)
	}

	const email = "dup-target@example.com"
	expires := time.Now().Add(72 * time.Hour)

	if _, err := store.CreateInvitation(ctx, org.ID, email, models.RoleViewer, "tokenhash-a", inviter.ID, expires); err != nil {
		t.Fatalf("first live invite: %v", err)
	}

	// A second *live* invite for the same (org, email) must be rejected.
	if _, err := store.CreateInvitation(ctx, org.ID, email, models.RoleViewer, "tokenhash-b", inviter.ID, expires); err == nil {
		t.Errorf("expected duplicate live invite to be rejected, got nil error")
	}
}
