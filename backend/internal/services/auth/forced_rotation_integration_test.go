//go:build integration
// +build integration

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authmodels "github.com/trakrf/platform/backend/internal/models/auth"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/password"
)

// TRA-1135: a forced-rotation flag is only useful if setting a new password
// takes it off. Both routes to a new password — the authenticated change
// (TRA-1130) and the emailed reset token — write through
// storage.UpdateUserPassword, so the clear belongs there and both are asserted
// here to keep it that way.

// seedFlaggedUser inserts a user already held at the change-password screen,
// the way an operator-provisioned account arrives.
func seedFlaggedUser(t *testing.T, pool *pgxpool.Pool, email, plaintext string) (int, string) {
	t.Helper()
	hash, err := password.Hash(plaintext)
	require.NoError(t, err)

	var userID int
	require.NoError(t, pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.users (name, email, password_hash, must_change_password)
		VALUES ($1, $2, $3, TRUE) RETURNING id`,
		"Forced Rotation Test", email, hash,
	).Scan(&userID))

	return userID, hash
}

func mustChangePassword(t *testing.T, pool *pgxpool.Pool, userID int) bool {
	t.Helper()
	var flag bool
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT must_change_password FROM trakrf.users WHERE id = $1`, userID).Scan(&flag))
	return flag
}

func TestChangePasswordClearsForcedRotation(t *testing.T) {
	t.Setenv("JWT_SECRET", "forced-rotation-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	userID, _ := seedFlaggedUser(t, pool, "forced-rotation-change@example.com", "bootstrap1")
	require.True(t, mustChangePassword(t, pool, userID), "fixture must start flagged")

	svc := NewService(pool, store, nil)

	// A refused attempt must leave the user exactly where they were — still
	// gated. Otherwise a wrong guess would open the app.
	err := svc.ChangePassword(ctx, userID, authmodels.ChangePasswordRequest{
		CurrentPassword: "not-the-password",
		NewPassword:     "chosen-pass1",
	}, password.Compare, password.Hash)
	require.ErrorIs(t, err, ErrInvalidCurrentPassword)
	assert.True(t, mustChangePassword(t, pool, userID), "a refused change must not clear the flag")

	require.NoError(t, svc.ChangePassword(ctx, userID, authmodels.ChangePasswordRequest{
		CurrentPassword: "bootstrap1",
		NewPassword:     "chosen-pass1",
	}, password.Compare, password.Hash))
	assert.False(t, mustChangePassword(t, pool, userID), "a successful change must clear the flag")
}

func TestResetPasswordClearsForcedRotation(t *testing.T) {
	t.Setenv("JWT_SECRET", "forced-rotation-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	userID, _ := seedFlaggedUser(t, pool, "forced-rotation-reset@example.com", "bootstrap1")

	// 64 hex chars, matching what generateResetToken emits — the column is
	// varchar(64) and rejects anything longer.
	const token = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	require.NoError(t, store.CreatePasswordResetToken(ctx, userID, token, time.Now().Add(time.Hour)))

	svc := NewService(pool, store, nil)
	require.NoError(t, svc.ResetPassword(ctx, token, "chosen-pass1", password.Hash))

	assert.False(t, mustChangePassword(t, pool, userID),
		"reset-via-email must clear the flag too — it is a password the user chose")
}

// TRA-1135 / TRA-1164: MCPSS's mail gateway quarantines our messages and
// releases them a day or two later, so a 24-hour single-use token was routinely
// dead before the recipient ever saw the link. 72h survives a slow gateway and
// is still far short of the invite flow's 7 days.
func TestForgotPasswordTokenLastsSeventyTwoHours(t *testing.T) {
	t.Setenv("JWT_SECRET", "forced-rotation-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	const email = "forced-rotation-ttl@example.com"
	userID, _ := seedFlaggedUser(t, pool, email, "bootstrap1")

	svc := NewService(pool, store, nil)
	issuedAt := time.Now()
	require.NoError(t, svc.ForgotPassword(ctx, email, "https://app.trakrf.id/#reset-password"))

	var expiresAt time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT expires_at FROM trakrf.password_reset_tokens WHERE user_id = $1`, userID).Scan(&expiresAt))

	ttl := expiresAt.Sub(issuedAt)
	assert.Greater(t, ttl, 71*time.Hour, "reset token TTL must be ~72h, not 24h")
	assert.Less(t, ttl, 73*time.Hour)
}
