//go:build integration
// +build integration

package auth_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/apikey"
	authservice "github.com/trakrf/platform/backend/internal/services/auth"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
	"github.com/trakrf/platform/backend/internal/util/jwt"
)

func newAPITokenService(t *testing.T) (*authservice.Service, *storage.Storage, *pgxpool.Pool, func()) {
	t.Helper()
	store, cleanup := testutil.SetupTestDB(t)
	pool := store.Pool().(*pgxpool.Pool)
	svc := authservice.NewService(pool, store, nil)
	return svc, store, pool, cleanup
}

func mkUser(t *testing.T, pool *pgxpool.Pool, email string) int {
	t.Helper()
	var id int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name,email,password_hash) VALUES ('u',$1,'x') RETURNING id`, email).Scan(&id))
	return id
}

func TestMintAPITokenPair_IssuesShortLivedJWT(t *testing.T) {
	svc, store, pool, cleanup := newAPITokenService(t)
	defer cleanup()
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := mkUser(t, pool, "apitok@example.com")
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read", "locations:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	access, refresh, expiresIn, err := svc.MintAPITokenPair(ctx, key.JTI, key.Scopes, orgID, int64(key.ID), "ua", "1.2.3.4")
	require.NoError(t, err)
	assert.NotEmpty(t, access)
	assert.NotEmpty(t, refresh)
	assert.Equal(t, 900, expiresIn) // 15 min

	claims, err := jwt.ValidateAccessToken(access)
	require.NoError(t, err)
	assert.Equal(t, key.JTI, claims.Subject)
	assert.Equal(t, orgID, claims.OrgID)
	assert.ElementsMatch(t, []string{"assets:read", "locations:read"}, claims.Scopes)
	require.NotNil(t, claims.ExpiresAt) // short-lived: exp is set
}

func TestRefreshAPIToken_RotatesWithCurrentScopes(t *testing.T) {
	svc, store, pool, cleanup := newAPITokenService(t)
	defer cleanup()
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := mkUser(t, pool, "apitok2@example.com")
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	_, refresh, _, err := svc.MintAPITokenPair(ctx, key.JTI, key.Scopes, orgID, int64(key.ID), "", "")
	require.NoError(t, err)

	resp, err := svc.RefreshAPIToken(ctx, refresh, "", "")
	require.NoError(t, err)
	assert.NotEmpty(t, resp.AccessToken)
	assert.NotEqual(t, refresh, resp.RefreshToken)
	assert.Equal(t, 900, resp.ExpiresIn)

	// Old refresh is now used → replay revokes the chain.
	_, err = svc.RefreshAPIToken(ctx, refresh, "", "")
	require.Error(t, err)
	// The freshly rotated token is also revoked by the chain-revoke.
	_, err = svc.RefreshAPIToken(ctx, resp.RefreshToken, "", "")
	require.Error(t, err)
}

func TestRefreshAPIToken_RejectsSessionToken(t *testing.T) {
	svc, _, pool, cleanup := newAPITokenService(t)
	defer cleanup()
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := mkUser(t, pool, "apitok3@example.com")

	// Mint a SESSION token pair, then present it at the API refresh endpoint.
	_, sessionRefresh, _, err := svc.MintTokenPair(ctx, userID, "apitok3@example.com", &orgID, "", "", jwt.Generate)
	require.NoError(t, err)

	_, err = svc.RefreshAPIToken(ctx, sessionRefresh, "", "")
	require.Error(t, err) // cross-type rejection
}

func TestRefreshAPIToken_RejectsRevokedKey(t *testing.T) {
	svc, store, pool, cleanup := newAPITokenService(t)
	defer cleanup()
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := mkUser(t, pool, "apitok4@example.com")
	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	_, refresh, _, err := svc.MintAPITokenPair(ctx, key.JTI, key.Scopes, orgID, int64(key.ID), "", "")
	require.NoError(t, err)

	require.NoError(t, store.RevokeAPIKey(ctx, orgID, key.ID))
	_, err = svc.RefreshAPIToken(ctx, refresh, "", "")
	require.Error(t, err) // key revoked → refresh rejected
}
