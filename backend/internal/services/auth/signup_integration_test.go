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

// TRA-971: self-service signup must persist the contact person's name + phone on
// the user, the company website on the org, and seed owner_user_id to the creating
// user (the org owner).
func TestSignup_SelfService_PersistsContactAndOwner(t *testing.T) {
	t.Setenv("JWT_SECRET", "signup-contact-test")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	svc := NewService(pool, store, nil)
	stubJWT := func(int, string, *int) (string, error) { return "stub-token", nil }
	stubHash := func(pw string) (string, error) { return "hashed-" + pw, nil }

	resp, err := svc.Signup(ctx, authmodels.SignupRequest{
		Email:    "contact-signup@example.com",
		Password: "s3cret!!",
		OrgName:  "Contact Org",
		Name:     "Jane Operator",
		Phone:    "+1-555-0100",
		Website:  "contact-org.example.com",
	}, "", "", stubHash, stubJWT)
	require.NoError(t, err)
	require.NotNil(t, resp)

	userID := resp.User.ID
	assert.Equal(t, "Jane Operator", resp.User.Name, "response user name must be the contact name, not the email")

	var (
		userName    string
		userPhone   *string
		orgWebsite  *string
		ownerUserID *int64
		orgID       int64
	)
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT u.name, u.phone, o.id, o.website, o.owner_user_id
		FROM trakrf.users u
		JOIN trakrf.org_users ou ON ou.user_id = u.id
		JOIN trakrf.organizations o ON o.id = ou.org_id
		WHERE u.id = $1
	`, userID).Scan(&userName, &userPhone, &orgID, &orgWebsite, &ownerUserID))

	assert.Equal(t, "Jane Operator", userName)
	require.NotNil(t, userPhone)
	assert.Equal(t, "+1-555-0100", *userPhone)
	require.NotNil(t, orgWebsite)
	assert.Equal(t, "contact-org.example.com", *orgWebsite)
	require.NotNil(t, ownerUserID, "owner_user_id must be seeded at signup")
	assert.Equal(t, int64(userID), *ownerUserID, "owner_user_id must point at the creating user")
}

// TRA-970: on a non-prod deployed environment, self-service signup is blocked
// with ErrSignupNotAllowed before any rows are written. The allowed-env path is
// already exercised by the other signup integration tests (APP_ENV unset).
func TestSignup_NonProdEnv_Blocked(t *testing.T) {
	t.Setenv("JWT_SECRET", "signup-envgate-test")
	t.Setenv("APP_ENV", "preview")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	svc := NewService(pool, store, nil)
	stubJWT := func(int, string, *int) (string, error) { return "stub-token", nil }
	stubHash := func(pw string) (string, error) { return "hashed-" + pw, nil }

	resp, err := svc.Signup(ctx, authmodels.SignupRequest{
		Email:    "blocked-signup@example.com",
		Password: "s3cret!!",
		OrgName:  "Blocked Org",
		Name:     "Nope",
		Phone:    "555-9999",
		Website:  "nope.example.com",
	}, "", "", stubHash, stubJWT)

	require.ErrorIs(t, err, ErrSignupNotAllowed)
	assert.Nil(t, resp)

	// No user row should have been created.
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.users WHERE email = $1`, "blocked-signup@example.com",
	).Scan(&count))
	assert.Equal(t, 0, count, "blocked signup must not create a user")
}

// TRA-970: a deliberate non-prod acknowledgment (AcknowledgeNonProd) lets signup
// proceed on a non-prod env — the warn-and-steer gate is a speed bump, not a wall.
func TestSignup_NonProdEnv_AckProceeds(t *testing.T) {
	t.Setenv("JWT_SECRET", "signup-ack-test")
	t.Setenv("APP_ENV", "preview")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	svc := NewService(pool, store, nil)
	stubJWT := func(int, string, *int) (string, error) { return "stub-token", nil }
	stubHash := func(pw string) (string, error) { return "hashed-" + pw, nil }

	resp, err := svc.Signup(ctx, authmodels.SignupRequest{
		Email:              "ack-signup@example.com",
		Password:           "s3cret!!",
		OrgName:            "Ack Org",
		Name:               "Yes Please",
		Phone:              "555-0001",
		Website:            "ack.example.com",
		AcknowledgeNonProd: true,
	}, "", "", stubHash, stubJWT)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "Yes Please", resp.User.Name)
}
