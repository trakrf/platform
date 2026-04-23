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

func createTestUser(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var id int
	err := pool.QueryRow(context.Background(), `
        INSERT INTO trakrf.users (name, email, password_hash)
        VALUES ($1, $2, $3) RETURNING id`,
		"test user", "testuser@example.com", "stub",
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestAPIKeyStorage_CreateAndGetByJTI(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)

	ctx := context.Background()
	scopes := []string{"assets:read", "locations:read"}
	key, err := store.CreateAPIKey(ctx, orgID, "test-key", scopes, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)
	assert.NotZero(t, key.ID)
	assert.NotEmpty(t, key.JTI)
	assert.Equal(t, orgID, key.OrgID)
	assert.Equal(t, "test-key", key.Name)

	got, err := store.GetAPIKeyByJTI(ctx, key.JTI)
	require.NoError(t, err)
	assert.Equal(t, key.ID, got.ID)
	assert.Equal(t, scopes, got.Scopes)
	assert.Nil(t, got.RevokedAt)
}

func TestAPIKeyStorage_ListExcludesRevoked(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	ctx := context.Background()

	active, err := store.CreateAPIKey(ctx, orgID, "active", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)
	revoked, err := store.CreateAPIKey(ctx, orgID, "revoked", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)
	require.NoError(t, store.RevokeAPIKey(ctx, orgID, revoked.ID))

	list, err := store.ListActiveAPIKeys(ctx, orgID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, active.ID, list[0].ID)
}

func TestAPIKeyStorage_CountActive(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
		require.NoError(t, err)
	}
	n, err := store.CountActiveAPIKeys(ctx, orgID)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestAPIKeyStorage_RevokeReturnsNotFoundForCrossOrg(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	org1 := testutil.CreateTestAccount(t, pool)
	var org2 int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active) VALUES ('Org 2', 'org-2', true) RETURNING id`,
	).Scan(&org2)
	require.NoError(t, err)

	userID := createTestUser(t, pool)
	ctx := context.Background()

	key, err := store.CreateAPIKey(ctx, org1, "org1-key", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	err = store.RevokeAPIKey(ctx, org2, key.ID)
	assert.ErrorIs(t, err, storage.ErrAPIKeyNotFound)
}

func TestAPIKeyStorage_UpdateLastUsed(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	ctx := context.Background()

	key, err := store.CreateAPIKey(ctx, orgID, "k", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)
	assert.Nil(t, key.LastUsedAt)

	err = store.UpdateAPIKeyLastUsed(ctx, key.JTI)
	require.NoError(t, err)

	got, err := store.GetAPIKeyByJTI(ctx, key.JTI)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
	assert.WithinDuration(t, time.Now(), *got.LastUsedAt, 5*time.Second)
}

func TestCreateAPIKey_WithCreatedByKeyID(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var seedUserID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name, email, password_hash) VALUES ('seed', 'seed@x', 'stub') RETURNING id`,
	).Scan(&seedUserID))

	parent, err := store.CreateAPIKey(context.Background(), orgID, "parent",
		[]string{"keys:admin"}, apikey.Creator{UserID: &seedUserID}, nil)
	require.NoError(t, err)

	child, err := store.CreateAPIKey(context.Background(), orgID, "child",
		[]string{"assets:read"}, apikey.Creator{KeyID: &parent.ID}, nil)
	require.NoError(t, err)
	require.Nil(t, child.CreatedBy)
	require.NotNil(t, child.CreatedByKeyID)
	assert.Equal(t, parent.ID, *child.CreatedByKeyID)

	// Roundtrip via List — creator fields survive scan.
	list, err := store.ListActiveAPIKeys(context.Background(), orgID)
	require.NoError(t, err)
	var roundtripped *apikey.APIKey
	for i := range list {
		if list[i].ID == child.ID {
			roundtripped = &list[i]
			break
		}
	}
	require.NotNil(t, roundtripped)
	assert.Nil(t, roundtripped.CreatedBy)
	require.NotNil(t, roundtripped.CreatedByKeyID)
	assert.Equal(t, parent.ID, *roundtripped.CreatedByKeyID)
}

// Direct SQL insert with both creator columns must violate the CHECK constraint.
func TestAPIKeys_CreatorExactlyOneCheck(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	var userID int
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.users (name, email, password_hash) VALUES ('u', 'u@x', 'stub') RETURNING id`,
	).Scan(&userID))

	parent, err := store.CreateAPIKey(context.Background(), orgID, "p",
		[]string{"keys:admin"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	// Bypass storage helper — raw INSERT with BOTH creator columns set → CHECK fails.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.api_keys (org_id, name, scopes, created_by, created_by_key_id)
		VALUES ($1, 'both', ARRAY['assets:read'], $2, $3)`,
		orgID, userID, parent.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_keys_creator_exactly_one")

	// And with NEITHER set → also CHECK fails.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.api_keys (org_id, name, scopes)
		VALUES ($1, 'neither', ARRAY['assets:read'])`, orgID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_keys_creator_exactly_one")
}
