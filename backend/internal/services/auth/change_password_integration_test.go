//go:build integration
// +build integration

package auth

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authmodels "github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/password"
)

// TRA-1130: ChangePassword must verify the current password against the
// stored hash before replacing it, and the replacement must be a hash the
// login path accepts.
func TestChangePassword_VerifiesCurrentAndRotates(t *testing.T) {
	t.Setenv("JWT_SECRET", "change-password-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	const email = "change-password-test@example.com"
	hash, err := password.Hash("oldpass123")
	require.NoError(t, err)

	var userID int
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO trakrf.users (name, email, password_hash)
		VALUES ($1, $2, $3) RETURNING id`,
		"Change Password Test", email, hash,
	).Scan(&userID))

	svc := NewService(pool, store, nil)

	// Wrong current password: refused with the sentinel, hash untouched.
	err = svc.ChangePassword(ctx, userID, authmodels.ChangePasswordRequest{
		CurrentPassword: "not-the-password",
		NewPassword:     "newpass123",
	}, password.Compare, password.Hash)
	require.ErrorIs(t, err, ErrInvalidCurrentPassword)

	var storedHash string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT password_hash FROM trakrf.users WHERE id = $1`, userID).Scan(&storedHash))
	assert.Equal(t, hash, storedHash, "a refused change must not touch the stored hash")

	// Correct current password: hash rotates and the new password verifies.
	err = svc.ChangePassword(ctx, userID, authmodels.ChangePasswordRequest{
		CurrentPassword: "oldpass123",
		NewPassword:     "newpass123",
	}, password.Compare, password.Hash)
	require.NoError(t, err)

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT password_hash FROM trakrf.users WHERE id = $1`, userID).Scan(&storedHash))
	assert.NotEqual(t, hash, storedHash)
	assert.NoError(t, password.Compare("newpass123", storedHash))
	assert.Error(t, password.Compare("oldpass123", storedHash))
}

// TRA-1130: a deleted (or never-existing) user must be refused, not crash.
func TestChangePassword_UnknownUser(t *testing.T) {
	t.Setenv("JWT_SECRET", "change-password-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	svc := NewService(pool, store, nil)

	err := svc.ChangePassword(context.Background(), 999999999, authmodels.ChangePasswordRequest{
		CurrentPassword: "whatever12",
		NewPassword:     "newpass123",
	}, password.Compare, password.Hash)
	require.Error(t, err)
}
