//go:build integration
// +build integration

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/organization"
	"github.com/trakrf/platform/backend/internal/services/email"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// TRA-967: a self-service trial signup notifies every active superadmin, and no
// one else. Reserved test-domain recipients keep the send stubbed (no Resend
// quota burned). notifyTrialSignup returns the count of superadmins notified.
func TestNotifyTrialSignup_NotifiesAllSuperadmins(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "dummy-never-used")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	for _, e := range []string{"ops1@example.com", "ops2@example.com"} {
		_, err := pool.Exec(ctx,
			`INSERT INTO trakrf.users (name, email, password_hash, is_superadmin)
			 VALUES ($1, $1, 'stub', true)`, e)
		require.NoError(t, err)
	}
	// A regular (non-superadmin) user must not be counted.
	_, err := pool.Exec(ctx,
		`INSERT INTO trakrf.users (name, email, password_hash, is_superadmin)
		 VALUES ('reg@example.com', 'reg@example.com', 'stub', false)`)
	require.NoError(t, err)

	svc := NewService(pool, store, email.NewClient())
	expires := time.Now().Add(720 * time.Hour)
	org := organization.Organization{
		Name:                  "Acme Co",
		Identifier:            "acme-co",
		SubscriptionExpiresAt: &expires,
	}

	sent := svc.notifyTrialSignup(ctx, org, "newuser@example.com")
	require.Equal(t, 2, sent, "should notify exactly the two superadmins")
}

// A nil emailClient (as wired in many tests) must be a no-op, never a panic.
func TestNotifyTrialSignup_NilClientIsNoOp(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	svc := NewService(pool, store, nil)
	sent := svc.notifyTrialSignup(context.Background(), organization.Organization{Name: "X", Identifier: "x"}, "u@example.com")
	require.Equal(t, 0, sent)
}
