//go:build integration
// +build integration

// TRA-659 / BB25 A3: GET /api/v1/assets?include_deleted=true returns
// soft-deleted rows alongside live rows, with asset_deleted_at populated
// for deleted rows and null for live rows. is_active and include_deleted
// are orthogonal toggles.

package assets

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestListAssets_IncludeDeleted_DefaultExcludesDeleted(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedAssetForFilter(t, pool, orgID, "LIVE-1", "Live one")
	deleted := seedAssetForFilter(t, pool, orgID, "DEAD-1", "Dead one")
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.assets SET deleted_at = now() WHERE id = $1`, deleted)
	require.NoError(t, err)

	router := setupExternalKeyListRouter(NewHandler(store))

	code, resp := doFilterRequest(t, router, orgID, "")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "LIVE-1", resp.Data[0].ExternalKey)
	assert.Nil(t, resp.Data[0].AssetDeletedAt, "live row asset_deleted_at must be null")
}

func TestListAssets_IncludeDeleted_True_SurfacesDeleted(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedAssetForFilter(t, pool, orgID, "LIVE-1", "Live one")
	deleted := seedAssetForFilter(t, pool, orgID, "DEAD-1", "Dead one")
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.assets SET deleted_at = now() WHERE id = $1`, deleted)
	require.NoError(t, err)

	router := setupExternalKeyListRouter(NewHandler(store))

	code, resp := doFilterRequest(t, router, orgID, "include_deleted=true")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Data, 2)
	require.Equal(t, 2, resp.TotalCount)

	byKey := map[string]int{}
	for i, a := range resp.Data {
		byKey[a.ExternalKey] = i
	}
	require.Contains(t, byKey, "LIVE-1")
	require.Contains(t, byKey, "DEAD-1")
	assert.Nil(t, resp.Data[byKey["LIVE-1"]].AssetDeletedAt, "live row asset_deleted_at must be null")
	assert.NotNil(t, resp.Data[byKey["DEAD-1"]].AssetDeletedAt, "deleted row asset_deleted_at must be populated")
}

func TestListAssets_IncludeDeleted_OrthogonalToIsActive(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	// Live + active
	seedAssetForFilter(t, pool, orgID, "LIVE-ACTIVE", "n")
	// Live + inactive
	id2 := seedAssetForFilter(t, pool, orgID, "LIVE-INACTIVE", "n")
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.assets SET is_active = false WHERE id = $1`, id2)
	require.NoError(t, err)
	// Deleted + active (was active when deleted)
	id3 := seedAssetForFilter(t, pool, orgID, "DEAD-ACTIVE", "n")
	_, err = pool.Exec(context.Background(),
		`UPDATE trakrf.assets SET deleted_at = now() WHERE id = $1`, id3)
	require.NoError(t, err)
	// Deleted + inactive
	id4 := seedAssetForFilter(t, pool, orgID, "DEAD-INACTIVE", "n")
	_, err = pool.Exec(context.Background(),
		`UPDATE trakrf.assets SET is_active = false, deleted_at = now() WHERE id = $1`, id4)
	require.NoError(t, err)

	router := setupExternalKeyListRouter(NewHandler(store))

	t.Run("is_active=false omitting include_deleted excludes deleted rows", func(t *testing.T) {
		code, resp := doFilterRequest(t, router, orgID, "is_active=false")
		require.Equal(t, http.StatusOK, code)
		require.Len(t, resp.Data, 1)
		assert.Equal(t, "LIVE-INACTIVE", resp.Data[0].ExternalKey)
	})

	t.Run("is_active=false&include_deleted=true returns inactive live + deleted rows", func(t *testing.T) {
		code, resp := doFilterRequest(t, router, orgID, "is_active=false&include_deleted=true")
		require.Equal(t, http.StatusOK, code)
		// Live inactive + deleted inactive (deleted+active stays out because is_active=false)
		keys := map[string]bool{}
		for _, a := range resp.Data {
			keys[a.ExternalKey] = true
		}
		assert.True(t, keys["LIVE-INACTIVE"])
		assert.True(t, keys["DEAD-INACTIVE"])
		assert.False(t, keys["LIVE-ACTIVE"])
		assert.False(t, keys["DEAD-ACTIVE"])
	})

	t.Run("is_active=true&include_deleted=true returns active live + deleted rows", func(t *testing.T) {
		code, resp := doFilterRequest(t, router, orgID, "is_active=true&include_deleted=true")
		require.Equal(t, http.StatusOK, code)
		keys := map[string]bool{}
		for _, a := range resp.Data {
			keys[a.ExternalKey] = true
		}
		assert.True(t, keys["LIVE-ACTIVE"])
		assert.True(t, keys["DEAD-ACTIVE"])
		assert.False(t, keys["LIVE-INACTIVE"])
		assert.False(t, keys["DEAD-INACTIVE"])
	})
}

func TestListAssets_IncludeDeleted_InvalidValue_400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupExternalKeyListRouter(NewHandler(store))

	code, _ := doFilterRequest(t, router, orgID, "include_deleted=banana")
	assert.Equal(t, http.StatusBadRequest, code)
}
