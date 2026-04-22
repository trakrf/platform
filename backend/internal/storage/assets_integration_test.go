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
	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestGetAssetByIdentifier_Found(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-1", Name: "Warehouse 1", Path: "wh-1",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "widget-42", Name: "Widget", Type: "asset",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	view, err := store.GetAssetByIdentifier(context.Background(), orgID, "widget-42")
	require.NoError(t, err)
	require.NotNil(t, view)
	assert.Equal(t, "widget-42", view.Identifier)
	require.NotNil(t, view.CurrentLocationIdentifier)
	assert.Equal(t, "wh-1", *view.CurrentLocationIdentifier)
}

func TestGetAssetByIdentifier_WrongOrg(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgA := testutil.CreateTestAccount(t, pool)

	// Create a second org with a distinct identifier (CreateTestAccount uses hardcoded "test-org").
	var orgB int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.organizations (name, identifier, is_active)
		VALUES ($1, $2, $3) RETURNING id
	`, "Org B", "test-org-b", true).Scan(&orgB)
	require.NoError(t, err)

	_, err = store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgA, Identifier: "a-only", Name: "A", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	view, err := store.GetAssetByIdentifier(context.Background(), orgB, "a-only")
	require.NoError(t, err)
	assert.Nil(t, view)
}

func TestGetAssetByIdentifier_SoftDeletedNotReturned(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	created, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "gone", Name: "Gone", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.DeleteAsset(context.Background(), orgID, created.ID)
	require.NoError(t, err)

	view, err := store.GetAssetByIdentifier(context.Background(), orgID, "gone")
	require.NoError(t, err)
	assert.Nil(t, view)
}

func TestListAssetsFiltered_LocationAndSort(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	locA, _ := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-A", Name: "A", Path: "wh-A",
		ValidFrom: time.Now(), IsActive: true,
	})
	locB, _ := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-B", Name: "B", Path: "wh-B",
		ValidFrom: time.Now(), IsActive: true,
	})

	for _, spec := range []struct {
		id   string
		name string
		loc  *int
	}{
		{"aaa", "A Asset", &locA.ID},
		{"bbb", "B Asset", &locB.ID},
		{"ccc", "C Asset", &locA.ID},
	} {
		_, err := store.CreateAsset(context.Background(), asset.Asset{
			OrgID: orgID, Identifier: spec.id, Name: spec.name, Type: "asset",
			CurrentLocationID: spec.loc, ValidFrom: time.Now(), IsActive: true,
		})
		require.NoError(t, err)
	}

	items, err := store.ListAssetsFiltered(context.Background(), orgID, asset.ListFilter{
		LocationIdentifiers: []string{"wh-A"},
		Sorts:               []asset.ListSort{{Field: "identifier", Desc: false}},
		Limit:               50, Offset: 0,
	})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "aaa", items[0].Identifier)
	assert.Equal(t, "ccc", items[1].Identifier)
	require.NotNil(t, items[0].CurrentLocationIdentifier)
	assert.Equal(t, "wh-A", *items[0].CurrentLocationIdentifier)

	count, err := store.CountAssetsFiltered(context.Background(), orgID, asset.ListFilter{
		LocationIdentifiers: []string{"wh-A"},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestListAssetsFiltered_Q(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	_, _ = store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "forklift-1", Name: "Forklift One", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	_, _ = store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "widget-1", Name: "Widget", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})

	q := "fork"
	items, err := store.ListAssetsFiltered(context.Background(), orgID, asset.ListFilter{
		Q: &q, Limit: 50,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "forklift-1", items[0].Identifier)
}

// TestGetAssetByIdentifier_CrossOrgLocationFenced defends the cross-tenant
// LEFT JOIN leak. An asset in org A whose current_location_id points at a
// location in org B (possible in theory via admin error, corrupt data, or a
// future cross-org move) must not expose the wrong-org location's natural
// identifier in the public response. The query's org fence on the LEFT JOIN
// ensures the current_location comes back as nil, not as the org B's name.
func TestGetAssetByIdentifier_CrossOrgLocationFenced(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgA := testutil.CreateTestAccount(t, pool)

	var orgB int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier) VALUES ($1, $2) RETURNING id`,
		"cross-org-test", "cross-org-test",
	).Scan(&orgB)
	require.NoError(t, err)

	// Location lives in org B.
	locB, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgB, Identifier: "org-b-location", Name: "B", Path: "org-b-location",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Asset lives in org A but its current_location_id points at org B's location.
	// CreateAsset enforces org context via RLS, so seed directly to simulate the
	// corrupted / cross-org state the fence defends against.
	var assetID int
	err = pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.assets
			(org_id, identifier, name, type, description, current_location_id,
			 valid_from, metadata, is_active, created_at, updated_at)
		 VALUES ($1, 'leaker', 'A', 'asset', '', $2, now(), '{}'::jsonb, true, now(), now())
		 RETURNING id`,
		orgA, locB.ID,
	).Scan(&assetID)
	require.NoError(t, err)

	view, err := store.GetAssetByIdentifier(context.Background(), orgA, "leaker")
	require.NoError(t, err)
	require.NotNil(t, view)
	assert.Equal(t, "leaker", view.Identifier)
	assert.Nil(t, view.CurrentLocationIdentifier,
		"LEFT JOIN must be fenced by org_id — wrong-org locations must not appear in current_location")

	// List variant: same asset should appear with nil current_location.
	items, err := store.ListAssetsFiltered(context.Background(), orgA, asset.ListFilter{Limit: 50})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Nil(t, items[0].CurrentLocationIdentifier)
}

// TestGetAssetWithLocationByID_ResolvesParent verifies that the private
// helper returns AssetWithLocation with CurrentLocationIdentifier populated
// when the asset has a live parent location, and nil when unset.
// Guards against regression to the bare Asset/AssetView shape on write paths.
func TestGetAssetWithLocationByID_ResolvesParent(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	// Create parent location inline
	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-1", Name: "Warehouse 1", Path: "wh-1",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Create asset with parent
	placed, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "tra429-placed", Name: "Placed", Type: "asset",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Happy path: parent identifier resolves
	got, err := store.GetAssetWithLocationByIDForTest(context.Background(), placed.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.CurrentLocationIdentifier)
	assert.Equal(t, "wh-1", *got.CurrentLocationIdentifier)
	assert.Equal(t, "tra429-placed", got.Identifier)

	// Create asset without parent
	unplaced, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "tra429-unplaced", Name: "Unplaced", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Negative: no parent → nil CurrentLocationIdentifier
	got2, err := store.GetAssetWithLocationByIDForTest(context.Background(), unplaced.ID)
	require.NoError(t, err)
	require.NotNil(t, got2)
	assert.Nil(t, got2.CurrentLocationIdentifier)
}

// TestGetAssetWithLocationByID_SoftDeletedAssetReturnsNil verifies the helper
// honors the `a.deleted_at IS NULL` predicate — a tombstoned asset must surface
// as (nil, nil), matching GetAssetByIdentifier's semantics.
func TestGetAssetWithLocationByID_SoftDeletedAssetReturnsNil(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	a, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "tra429-doomed", Name: "Doomed", Type: "asset",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Soft-delete via the storage method (same path production uses).
	deleted, err := store.DeleteAsset(context.Background(), orgID, a.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	got, err := store.GetAssetWithLocationByIDForTest(context.Background(), a.ID)
	require.NoError(t, err)
	assert.Nil(t, got, "soft-deleted asset should surface as nil, not the stale row")
}

// TestGetAssetWithLocationByID_SoftDeletedLocationYieldsNilIdentifier verifies
// the LEFT JOIN's `l.deleted_at IS NULL` predicate — a live asset pointing at
// a tombstoned location should expose nil CurrentLocationIdentifier, never
// the stale identifier.
func TestGetAssetWithLocationByID_SoftDeletedLocationYieldsNilIdentifier(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "tra429-loc-tombstone", Name: "Tombstone",
		Path: "tra429-loc-tombstone", ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	a, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "tra429-stale-ref", Name: "StaleRef", Type: "asset",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Soft-delete the parent location, leaving the FK dangling.
	deleted, err := store.DeleteLocation(context.Background(), orgID, loc.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	got, err := store.GetAssetWithLocationByIDForTest(context.Background(), a.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.CurrentLocationIdentifier,
		"LEFT JOIN's deleted_at IS NULL predicate must suppress the stale parent identifier")
}

// TestGetAssetWithLocationByID_UnknownIDReturnsNil verifies the (nil, nil)
// sentinel on pgx.ErrNoRows for a surrogate id that names no asset.
func TestGetAssetWithLocationByID_UnknownIDReturnsNil(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	got, err := store.GetAssetWithLocationByIDForTest(context.Background(), 99999999)
	require.NoError(t, err)
	assert.Nil(t, got)
}
