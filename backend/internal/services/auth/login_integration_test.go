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

// TRA-449 D8: Login must populate user.last_login_at on the returned payload,
// and persist it to trakrf.users so subsequent reads observe the same value.
func TestLogin_PopulatesLastLoginAt(t *testing.T) {
	t.Setenv("JWT_SECRET", "login-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)

	const email = "login-test@example.com"
	hash, err := password.Hash("s3cret!!")
	require.NoError(t, err)

	var userID int
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO trakrf.users (name, email, password_hash)
		VALUES ($1, $2, $3) RETURNING id`,
		"Login Test", email, hash,
	).Scan(&userID))

	_, err = pool.Exec(ctx, `
		INSERT INTO trakrf.org_users (org_id, user_id, role, status)
		VALUES ($1, $2, 'admin', 'active')`, orgID, userID)
	require.NoError(t, err)

	svc := NewService(pool, store, nil)

	stubJWT := func(int, string, *int) (string, error) { return "stub-token", nil }
	before := time.Now().UTC()
	resp, err := svc.Login(ctx, authmodels.LoginRequest{Email: email, Password: "s3cret!!"},
		password.Compare, stubJWT)
	require.NoError(t, err)
	after := time.Now().UTC()

	require.NotNil(t, resp.User.LastLoginAt, "login response must set user.last_login_at")
	assert.WithinDuration(t, after, *resp.User.LastLoginAt, after.Sub(before)+2*time.Second)

	// Persistence: a direct read of trakrf.users sees the same timestamp.
	var persisted *time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT last_login_at FROM trakrf.users WHERE id = $1`, userID,
	).Scan(&persisted))
	require.NotNil(t, persisted, "users.last_login_at must be persisted")
	assert.True(t, resp.User.LastLoginAt.Equal(*persisted),
		"response timestamp %v must equal persisted %v", resp.User.LastLoginAt, persisted)

	// A second login advances the timestamp — proves we're not returning a cached value.
	time.Sleep(20 * time.Millisecond)
	resp2, err := svc.Login(ctx, authmodels.LoginRequest{Email: email, Password: "s3cret!!"},
		password.Compare, stubJWT)
	require.NoError(t, err)
	require.NotNil(t, resp2.User.LastLoginAt)
	assert.True(t, resp2.User.LastLoginAt.After(*resp.User.LastLoginAt),
		"second login timestamp %v must be after first %v", resp2.User.LastLoginAt, resp.User.LastLoginAt)
}
