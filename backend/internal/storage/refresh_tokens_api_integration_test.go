//go:build integration
// +build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/apikey"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// An api-type refresh row: user_id NULL, api_key_id set — must be allowed
// after migration 000012.
func TestRefreshTokens_APIRowAllowsNullUser(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(context.Background(), orgID, "k", "testhash", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	var id int64
	err = pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.refresh_tokens (token_type, user_id, org_id, api_key_id, token_hash, expires_at)
		VALUES ('api', NULL, $1, $2, $3, $4) RETURNING id`,
		orgID, key.ID, "hash_api_row_1", time.Now().Add(time.Hour),
	).Scan(&id)
	require.NoError(t, err)
	assert.NotZero(t, id)
}

// The tightened CHECK must reject an api row that still carries a user_id.
func TestRefreshTokens_APIRowRejectsUser(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(context.Background(), orgID, "k", "testhash", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.refresh_tokens (token_type, user_id, org_id, api_key_id, token_hash, expires_at)
		VALUES ('api', $1, $2, $3, $4, $5)`,
		userID, orgID, key.ID, "hash_api_bad", time.Now().Add(time.Hour),
	)
	require.Error(t, err) // violates refresh_tokens_type_consistent
}

func TestStorage_CreateAndGetAPIRefreshToken(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(ctx, orgID, "k", "testhash", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	id, err := store.CreateAPIRefreshToken(ctx, int64(key.ID), &orgID, "hash_get_1", time.Now().Add(time.Hour), "ua", "1.2.3.4")
	require.NoError(t, err)
	assert.NotZero(t, id)

	row, err := store.GetRefreshTokenByHash(ctx, "hash_get_1")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, "api", row.TokenType)
	assert.Nil(t, row.UserID)
	require.NotNil(t, row.APIKeyID)
	assert.Equal(t, int64(key.ID), *row.APIKeyID)
}

func TestStorage_RotateAPIRefreshToken(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(ctx, orgID, "k", "testhash", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	oldID, err := store.CreateAPIRefreshToken(ctx, int64(key.ID), &orgID, "hash_rot_old", time.Now().Add(time.Hour), "", "")
	require.NoError(t, err)

	newID, err := store.RotateAPIRefreshToken(ctx, oldID, int64(key.ID), &orgID, "hash_rot_new", time.Now().Add(time.Hour), "", "")
	require.NoError(t, err)
	assert.NotEqual(t, oldID, newID)

	old, err := store.GetRefreshTokenByHash(ctx, "hash_rot_old")
	require.NoError(t, err)
	assert.NotNil(t, old.UsedAt)
	require.NotNil(t, old.ReplacedBy)
	assert.Equal(t, newID, *old.ReplacedBy)
}

func TestStorage_GetAPIKeyByID(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(ctx, orgID, "k", "testhash", []string{"assets:read", "locations:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	got, err := store.GetAPIKeyByID(ctx, int64(key.ID))
	require.NoError(t, err)
	assert.Equal(t, key.JTI, got.JTI)
	assert.Equal(t, []string{"assets:read", "locations:read"}, got.Scopes)

	_, err = store.GetAPIKeyByID(ctx, 999999999)
	require.ErrorIs(t, err, storage.ErrAPIKeyNotFound)
}

func TestStorage_APIKeyDeleteCascadesRefreshTokens(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	key, err := store.CreateAPIKey(ctx, orgID, "k", "testhash", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)
	_, err = store.CreateAPIRefreshToken(ctx, int64(key.ID), &orgID, "hash_cascade", time.Now().Add(time.Hour), "", "")
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM trakrf.api_keys WHERE id = $1`, key.ID)
	require.NoError(t, err)

	row, err := store.GetRefreshTokenByHash(ctx, "hash_cascade")
	require.NoError(t, err)
	assert.Nil(t, row, "refresh token should be CASCADE-deleted with its api_keys row")
}
