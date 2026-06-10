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
)

// TRA-947 T8: self-service signup must start a 1-month trial (subscription_expires_at ≈ now()+1mo).
// Invitation-based signup and CreateOrgWithAdmin stay perpetual (NULL).
func TestSignup_SelfService_SetsOneMonthTrial(t *testing.T) {
	t.Setenv("JWT_SECRET", "signup-trial-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	svc := NewService(pool, store, nil)

	stubJWT := func(int, string, *int) (string, error) { return "stub-token", nil }
	stubHash := func(pw string) (string, error) { return "hashed-" + pw, nil }

	before := time.Now().UTC()
	resp, err := svc.Signup(ctx, authmodels.SignupRequest{
		Email:    "trial-signup@example.com",
		Password: "s3cret!!",
		OrgName:  "Trial Org",
	}, "", "", stubHash, stubJWT)
	require.NoError(t, err)
	require.NotNil(t, resp)
	after := time.Now().UTC()

	userID := resp.User.ID

	// Query subscription_expires_at via the admin pool (superuser bypasses RLS).
	var expiresAt *time.Time
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT subscription_expires_at
		FROM trakrf.organizations
		WHERE id = (
			SELECT org_id FROM trakrf.org_users WHERE user_id = $1 LIMIT 1
		)
	`, userID).Scan(&expiresAt))

	require.NotNil(t, expiresAt, "self-service signup must set subscription_expires_at (1-month trial)")

	// Should be roughly 1 month out: between 27 and 32 days from now.
	minExpiry := before.AddDate(0, 0, 27)
	maxExpiry := after.AddDate(0, 0, 32)
	assert.True(t, expiresAt.After(minExpiry),
		"subscription_expires_at %v should be after %v (27d from before)", expiresAt, minExpiry)
	assert.True(t, expiresAt.Before(maxExpiry),
		"subscription_expires_at %v should be before %v (32d from after)", expiresAt, maxExpiry)
}
