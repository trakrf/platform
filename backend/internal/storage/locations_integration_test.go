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
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestGetLocationByExternalKey_Found(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	parent, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "wh-1", Name: "Warehouse 1", Path: "wh-1",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "wh-1.bay-3", Name: "Bay 3",
		ParentID: &parent.ID,
		Path:     "wh-1.bay-3", ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	view, err := store.GetLocationByExternalKey(context.Background(), orgID, "wh-1.bay-3")
	require.NoError(t, err)
	require.NotNil(t, view)
	assert.Equal(t, "wh-1.bay-3", view.ExternalKey)
	require.NotNil(t, view.ParentExternalKey)
	assert.Equal(t, "wh-1", *view.ParentExternalKey)
}

func TestGetLocationByExternalKey_NotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	view, err := store.GetLocationByExternalKey(context.Background(), orgID, "missing")
	require.NoError(t, err)
	assert.Nil(t, view)
}

func TestListLocationsFiltered_Parent(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	root, _ := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "root", Name: "R", Path: "root",
		ValidFrom: time.Now(), IsActive: true,
	})
	_, _ = store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "root.a", Name: "A", ParentID: &root.ID,
		Path: "root.a", ValidFrom: time.Now(), IsActive: true,
	})
	_, _ = store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "root.b", Name: "B", ParentID: &root.ID,
		Path: "root.b", ValidFrom: time.Now(), IsActive: true,
	})

	items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
		ParentExternalKeys: []string{"root"},
		Sorts:              []location.ListSort{{Field: "external_key"}},
		Limit:              50,
	})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "root.a", items[0].ExternalKey)
	assert.Equal(t, "root.b", items[1].ExternalKey)
	require.NotNil(t, items[0].ParentExternalKey)
	assert.Equal(t, "root", *items[0].ParentExternalKey)
}

func TestListLocationsFiltered_Integration_ExternalKeysNeverNil(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	// One location with no tags, one with a tag.
	_, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "loc-empty", Name: "Empty", Path: "loc-empty",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	withTag, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "loc-tagged", Name: "Tagged", Path: "loc-tagged",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Insert a tag for withTag. Table: trakrf.tags,
	// columns: org_id, type, value, location_id, valid_from, is_active.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.tags (org_id, type, value, location_id, valid_from, is_active)
		VALUES ($1, 'rfid', $2, $3, NOW(), true)
	`, orgID, "EPC-TAGGED", withTag.ID)
	require.NoError(t, err)

	items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
		Sorts: []location.ListSort{{Field: "external_key"}},
		Limit: 50,
	})
	require.NoError(t, err)
	require.Len(t, items, 2)

	for _, item := range items {
		require.NotNil(t, item.Tags,
			"location %q ExternalKeys should not be nil (JSON would marshal to null)", item.ExternalKey)
	}

	var empty, tagged *location.LocationWithParent
	for i := range items {
		switch items[i].ExternalKey {
		case "loc-empty":
			empty = &items[i]
		case "loc-tagged":
			tagged = &items[i]
		}
	}
	require.NotNil(t, empty)
	require.NotNil(t, tagged)
	assert.Empty(t, empty.Tags, "loc-empty should have zero tags")
	assert.Len(t, tagged.Tags, 1)
	assert.Equal(t, "EPC-TAGGED", tagged.Tags[0].Value)
}

// TestGetLocationWithParentByID_ResolvesParent verifies that the private
// helper returns LocationWithParent with ParentIdentifier populated when the
// location has a live parent, and nil when the location is root-level.
// Guards against regression to the bare Location/LocationView shape on write
// paths.
func TestGetLocationWithParentByID_ResolvesParent(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	// Create parent location inline
	parent, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "wh-1", Name: "Warehouse 1", Path: "wh-1",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Create child with parent
	child, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "wh-1.bay-3", Name: "Bay 3",
		ParentID: &parent.ID,
		Path:     "wh-1.bay-3", ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Happy path: parent identifier resolves
	got, err := store.GetLocationWithParentByIDForTest(context.Background(), orgID, child.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.ParentExternalKey)
	assert.Equal(t, "wh-1", *got.ParentExternalKey)
	assert.Equal(t, "wh-1.bay-3", got.ExternalKey)

	// Create a root-level location (no parent)
	root, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "wh-2", Name: "Warehouse 2", Path: "wh-2",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Negative: no parent → nil ParentIdentifier
	got2, err := store.GetLocationWithParentByIDForTest(context.Background(), orgID, root.ID)
	require.NoError(t, err)
	require.NotNil(t, got2)
	assert.Nil(t, got2.ParentExternalKey)
}

// TestGetLocationWithParentByID_SoftDeletedLocationReturnsNil verifies the
// helper honors the `l.deleted_at IS NULL` predicate — a tombstoned location
// must surface as (nil, nil), matching GetLocationByExternalKey's semantics.
func TestGetLocationWithParentByID_SoftDeletedLocationReturnsNil(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "tra429-loc-doomed", Name: "Doomed",
		Path: "tra429-loc-doomed", ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Soft-delete via the storage method (same path production uses).
	deleted, err := store.DeleteLocation(context.Background(), orgID, loc.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	got, err := store.GetLocationWithParentByIDForTest(context.Background(), orgID, loc.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "soft-deleted location should surface as nil, not the stale row")
}

// TestGetLocationWithParentByID_SoftDeletedParentYieldsNilIdentifier verifies
// the LEFT JOIN's `p.deleted_at IS NULL` predicate — a live child pointing at
// a tombstoned parent should expose nil ParentIdentifier, never the stale
// identifier.
func TestGetLocationWithParentByID_SoftDeletedParentYieldsNilIdentifier(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	parent, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "tra429-parent-tombstone", Name: "ParentTombstone",
		Path: "tra429-parent-tombstone", ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	child, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "tra429-stale-child", Name: "StaleChild",
		ParentID: &parent.ID,
		Path:     "tra429-stale-child", ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Soft-delete the parent location, leaving the FK dangling.
	deleted, err := store.DeleteLocation(context.Background(), orgID, parent.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	got, err := store.GetLocationWithParentByIDForTest(context.Background(), orgID, child.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.ParentExternalKey,
		"LEFT JOIN's deleted_at IS NULL predicate must suppress the stale parent identifier")
}

// TestGetLocationWithParentByID_UnknownIDReturnsNil verifies the (nil, nil)
// sentinel on pgx.ErrNoRows for a surrogate id that names no location.
func TestGetLocationWithParentByID_UnknownIDReturnsNil(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	got, err := store.GetLocationWithParentByIDForTest(context.Background(), 0, 99999999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestUpdateLocation_PopulatesParentIdentifier verifies UpdateLocation
// returns the LocationWithParent shape with ParentIdentifier populated
// when the location has a live parent (TRA-429).
func TestUpdateLocation_PopulatesParentIdentifier(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	parent, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "wh-1", Name: "Warehouse 1", Path: "wh-1",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	child, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "wh-1.bay-3", Name: "Bay 3",
		Path: "wh-1.bay-3", ParentID: &parent.ID,
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	newName := "updated for tra-429"
	result, err := store.UpdateLocation(context.Background(), orgID, child.ID, location.UpdateLocationRequest{
		Name: &newName,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.ParentExternalKey)
	assert.Equal(t, "wh-1", *result.ParentExternalKey)
	assert.Equal(t, newName, result.Name)
	assert.NotNil(t, result.Tags, "Tags slice must be non-nil (empty is OK)")
}

func TestListLocationsFiltered_Q(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	activeLoc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "loc-active", Name: "Warehouse Active", Path: "loc-active",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	inactiveIDLoc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "loc-inactive-id", Name: "InactiveID", Path: "loc-inactive-id",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	deletedIDLoc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, ExternalKey: "loc-deleted-id", Name: "DeletedID", Path: "loc-deleted-id",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.tags (org_id, type, value, location_id, valid_from, is_active)
		VALUES ($1, 'rfid', 'LOC-ACTIVE-20055', $2, NOW(), true)
	`, orgID, activeLoc.ID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.tags (org_id, type, value, location_id, valid_from, is_active)
		VALUES ($1, 'rfid', 'LOC-INACTIVE-20055', $2, NOW(), false)
	`, orgID, inactiveIDLoc.ID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.tags (org_id, type, value, location_id, valid_from, is_active, deleted_at)
		VALUES ($1, 'rfid', 'LOC-DELETED-20055', $2, NOW(), true, NOW())
	`, orgID, deletedIDLoc.ID)
	require.NoError(t, err)

	t.Run("name substring matches", func(t *testing.T) {
		q := "Warehouse"
		items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "loc-active", items[0].ExternalKey)
	})

	t.Run("active identifier value matches", func(t *testing.T) {
		q := "20055"
		items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "loc-active", items[0].ExternalKey)
	})

	t.Run("inactive identifier value does not match", func(t *testing.T) {
		q := "INACTIVE-20055"
		items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		assert.Empty(t, items)
	})

	t.Run("soft-deleted identifier value does not match", func(t *testing.T) {
		q := "DELETED-20055"
		items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		assert.Empty(t, items)
	})
}

func TestLocationsPartialUniqueIndex_BlocksLiveDuplicates(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)

	insert := func(identifier string) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO trakrf.locations (org_id, external_key, name, valid_from)
			VALUES ($1, $2, 'name', now())
		`, orgID, identifier)
		return err
	}

	require.NoError(t, insert("part-idx-loc-1"))

	err := insert("part-idx-loc-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate key")

	_, err = pool.Exec(ctx, `
		UPDATE trakrf.locations SET deleted_at = now()
		 WHERE org_id = $1 AND external_key = $2
	`, orgID, "part-idx-loc-1")
	require.NoError(t, err)

	require.NoError(t, insert("part-idx-loc-1"), "should be allowed after soft-delete")
}
