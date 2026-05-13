//go:build integration
// +build integration

// TRA-659 / BB25 A3: GET /api/v1/locations?include_deleted=true returns
// soft-deleted rows alongside live rows, with deleted_at populated
// for deleted rows and null for live rows (TRA-679 / BB27 S7 renamed the
// per-resource field from location_deleted_at to deleted_at). is_active
// and include_deleted are orthogonal toggles.

package locations

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestListLocations_IncludeDeleted_DefaultExcludesDeleted(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedLocationForFilter(t, pool, orgID, "LIVE-1", "Live one")
	deleted := seedLocationForFilter(t, pool, orgID, "DEAD-1", "Dead one")
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.locations SET deleted_at = now() WHERE id = $1`, deleted)
	require.NoError(t, err)

	router := setupLocFilterRouter(NewHandler(store))

	code, resp := doLocFilterRequest(t, router, orgID, "")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "LIVE-1", resp.Data[0].ExternalKey)
	assert.Nil(t, resp.Data[0].DeletedAt, "live row deleted_at must be null")
}

func TestListLocations_IncludeDeleted_True_SurfacesDeleted(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedLocationForFilter(t, pool, orgID, "LIVE-1", "Live one")
	deleted := seedLocationForFilter(t, pool, orgID, "DEAD-1", "Dead one")
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.locations SET deleted_at = now() WHERE id = $1`, deleted)
	require.NoError(t, err)

	router := setupLocFilterRouter(NewHandler(store))

	code, resp := doLocFilterRequest(t, router, orgID, "include_deleted=true")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Data, 2)
	require.Equal(t, 2, resp.TotalCount)

	byKey := map[string]int{}
	for i, l := range resp.Data {
		byKey[l.ExternalKey] = i
	}
	require.Contains(t, byKey, "LIVE-1")
	require.Contains(t, byKey, "DEAD-1")
	assert.Nil(t, resp.Data[byKey["LIVE-1"]].DeletedAt, "live row deleted_at must be null")
	assert.NotNil(t, resp.Data[byKey["DEAD-1"]].DeletedAt, "deleted row deleted_at must be populated")
}

func TestListLocations_IncludeDeleted_OrthogonalToIsActive(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	seedLocationForFilter(t, pool, orgID, "LIVE-ACTIVE", "n")
	id2 := seedLocationForFilter(t, pool, orgID, "LIVE-INACTIVE", "n")
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.locations SET is_active = false WHERE id = $1`, id2)
	require.NoError(t, err)
	id3 := seedLocationForFilter(t, pool, orgID, "DEAD-ACTIVE", "n")
	_, err = pool.Exec(context.Background(),
		`UPDATE trakrf.locations SET deleted_at = now() WHERE id = $1`, id3)
	require.NoError(t, err)
	id4 := seedLocationForFilter(t, pool, orgID, "DEAD-INACTIVE", "n")
	_, err = pool.Exec(context.Background(),
		`UPDATE trakrf.locations SET is_active = false, deleted_at = now() WHERE id = $1`, id4)
	require.NoError(t, err)

	router := setupLocFilterRouter(NewHandler(store))

	t.Run("is_active=false omitting include_deleted excludes deleted rows", func(t *testing.T) {
		code, resp := doLocFilterRequest(t, router, orgID, "is_active=false")
		require.Equal(t, http.StatusOK, code)
		require.Len(t, resp.Data, 1)
		assert.Equal(t, "LIVE-INACTIVE", resp.Data[0].ExternalKey)
	})

	t.Run("is_active=false&include_deleted=true returns inactive live + deleted rows", func(t *testing.T) {
		code, resp := doLocFilterRequest(t, router, orgID, "is_active=false&include_deleted=true")
		require.Equal(t, http.StatusOK, code)
		keys := map[string]bool{}
		for _, l := range resp.Data {
			keys[l.ExternalKey] = true
		}
		assert.True(t, keys["LIVE-INACTIVE"])
		assert.True(t, keys["DEAD-INACTIVE"])
		assert.False(t, keys["LIVE-ACTIVE"])
		assert.False(t, keys["DEAD-ACTIVE"])
	})

	t.Run("is_active=true&include_deleted=true returns active live + deleted rows", func(t *testing.T) {
		code, resp := doLocFilterRequest(t, router, orgID, "is_active=true&include_deleted=true")
		require.Equal(t, http.StatusOK, code)
		keys := map[string]bool{}
		for _, l := range resp.Data {
			keys[l.ExternalKey] = true
		}
		assert.True(t, keys["LIVE-ACTIVE"])
		assert.True(t, keys["DEAD-ACTIVE"])
		assert.False(t, keys["LIVE-INACTIVE"])
		assert.False(t, keys["DEAD-INACTIVE"])
	})
}

func TestListLocations_IncludeDeleted_InvalidValue_400(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	router := setupLocFilterRouter(NewHandler(store))

	code, _ := doLocFilterRequest(t, router, orgID, "include_deleted=banana")
	assert.Equal(t, http.StatusBadRequest, code)
}

// TRA-693 / BB30 §2.6: with include_deleted=true, a soft-deleted child must
// still project its parent_external_key — even when the parent is also
// soft-deleted — because the value lives on the parent row and is useful to
// integrators reconciling tombstoned hierarchies.
func TestListLocations_IncludeDeleted_SoftDeletedParent_ChildProjectsParentExternalKey(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	parentID := seedLocationForFilter(t, pool, orgID, "tra693-parent", "Parent")
	childID := seedLocationForFilter(t, pool, orgID, "tra693-child", "Child")
	_, err := pool.Exec(context.Background(),
		`UPDATE trakrf.locations SET parent_location_id = $1 WHERE id = $2`, parentID, childID)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(),
		`UPDATE trakrf.locations SET deleted_at = now() WHERE id IN ($1, $2)`, parentID, childID)
	require.NoError(t, err)

	router := setupLocFilterRouter(NewHandler(store))

	code, resp := doLocFilterRequest(t, router, orgID, "include_deleted=true&external_key=tra693-child")
	require.Equal(t, http.StatusOK, code)
	require.Len(t, resp.Data, 1, "soft-deleted child must surface under include_deleted=true")
	assert.NotNil(t, resp.Data[0].DeletedAt, "child deleted_at must be populated")
	require.NotNil(t, resp.Data[0].ParentExternalKey,
		"parent_external_key must reflect the parent's external_key even when the parent is also soft-deleted")
	assert.Equal(t, "tra693-parent", *resp.Data[0].ParentExternalKey)
}
