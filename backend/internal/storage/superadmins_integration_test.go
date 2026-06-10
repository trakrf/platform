//go:build integration

package storage_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// seedUser inserts a user with an explicit is_superadmin flag and returns its id.
func seedUser(t *testing.T, pool *pgxpool.Pool, email string, superadmin bool) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name, email, password_hash, is_superadmin)
		 VALUES ($1, $2, 'stub', $3) RETURNING id`,
		email, email, superadmin).Scan(&id)
	if err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

// TRA-967: ListSuperadmins returns only active is_superadmin users (cross-org),
// excluding regular users and soft-deleted superadmins, ordered by email.
func TestListSuperadmins_ReturnsOnlyActiveSuperadmins(t *testing.T) {
	store := testutil.SetupTestDatabase(t)
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)

	seedUser(t, pool, "z-admin@example.com", true)
	seedUser(t, pool, "a-admin@example.com", true)
	seedUser(t, pool, "regular@example.com", false)
	deletedID := seedUser(t, pool, "gone-admin@example.com", true)
	if _, err := pool.Exec(ctx,
		`UPDATE trakrf.users SET deleted_at = now() WHERE id = $1`, deletedID); err != nil {
		t.Fatalf("soft-delete: %v", err)
	}

	admins, err := store.ListSuperadmins(ctx)
	if err != nil {
		t.Fatalf("ListSuperadmins: %v", err)
	}

	emails := make([]string, len(admins))
	for i, a := range admins {
		emails[i] = a.Email
		if !a.IsSuperadmin {
			t.Errorf("%s returned with IsSuperadmin=false", a.Email)
		}
	}

	if len(admins) != 2 {
		t.Fatalf("got %d superadmins %v, want 2", len(admins), emails)
	}
	// Ordered by email ascending.
	if emails[0] != "a-admin@example.com" || emails[1] != "z-admin@example.com" {
		t.Errorf("order = %v, want [a-admin@example.com, z-admin@example.com]", emails)
	}
}
