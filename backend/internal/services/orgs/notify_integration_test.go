//go:build integration
// +build integration

package orgs

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

// seedSuperadmins inserts n superadmins (reserved test-domain emails so sends
// are stubbed) plus one regular user that must never be notified.
func seedSuperadmins(t *testing.T, pool *pgxpool.Pool, emails ...string) {
	t.Helper()
	ctx := context.Background()
	for _, e := range emails {
		_, err := pool.Exec(ctx,
			`INSERT INTO trakrf.users (name, email, password_hash, is_superadmin)
			 VALUES ($1, $1, 'stub', true)`, e)
		require.NoError(t, err)
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO trakrf.users (name, email, password_hash, is_superadmin)
		 VALUES ('reg@example.com', 'reg@example.com', 'stub', false)`)
	require.NoError(t, err)
}

// TRA-977: an internal org create notifies every superadmin, and no one else.
func TestNotifyOrgCreated_NotifiesAllSuperadmins(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "dummy-never-used")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	seedSuperadmins(t, pool, "ops1@example.com", "ops2@example.com")

	svc := NewService(pool, store, email.NewClient())
	org := organization.Organization{Name: "Acme Co", Identifier: "acme-co"}

	sent := svc.notifyOrgCreated(context.Background(), org, "creator@example.com")
	require.Equal(t, 2, sent, "should notify exactly the two superadmins")
}

// TRA-977: an org delete notifies every superadmin (churn postmortem signal).
func TestNotifyOrgDeleted_NotifiesAllSuperadmins(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "dummy-never-used")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	seedSuperadmins(t, pool, "ops1@example.com", "ops2@example.com")

	svc := NewService(pool, store, email.NewClient())

	sent := svc.notifyOrgDeleted(context.Background(), "Acme Co", "acme-co", "actor@example.com", time.Now())
	require.Equal(t, 2, sent, "should notify exactly the two superadmins")
}

// A nil emailClient must be a no-op, never a panic.
func TestNotifyOrg_NilClientIsNoOp(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	svc := NewService(pool, store, nil)
	require.Equal(t, 0, svc.notifyOrgCreated(context.Background(), organization.Organization{Name: "X", Identifier: "x"}, "c@example.com"))
	require.Equal(t, 0, svc.notifyOrgDeleted(context.Background(), "X", "x", "a@example.com", time.Now()))
}
