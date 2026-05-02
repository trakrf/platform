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
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// mkLoc inserts a location through the storage API so triggers fire as in
// production. Returns the created row (with assigned id and canonical path).
func mkLoc(t *testing.T, ctx context.Context, store *storage.Storage, orgID int, externalKey string, parentID *int) *location.Location {
	t.Helper()
	loc, err := store.CreateLocation(ctx, location.Location{
		OrgID:       orgID,
		ExternalKey: externalKey,
		Name:        externalKey,
		ParentID:    parentID,
		ValidFrom:   time.Now(),
		IsActive:    true,
	})
	require.NoError(t, err)
	require.NotNil(t, loc)
	return loc
}

// TestUpdateLocation_Reparent_CascadesDescendants exercises the BB15 cascade
// gotcha: moving B (parent of C) under D must update C.path too.
func TestUpdateLocation_Reparent_CascadesDescendants(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	a := mkLoc(t, ctx, store, orgID, "a", nil)
	b := mkLoc(t, ctx, store, orgID, "b", &a.ID)
	c := mkLoc(t, ctx, store, orgID, "c", &b.ID)
	d := mkLoc(t, ctx, store, orgID, "d", nil)

	require.Equal(t, "a", a.Path)
	require.Equal(t, "a.b", b.Path)
	require.Equal(t, "a.b.c", c.Path)
	require.Equal(t, "d", d.Path)

	updated, err := store.UpdateLocation(ctx, orgID, b.ID, location.UpdateLocationRequest{
		ParentID: &d.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "d.b", updated.Path, "B's own path must reflect new parent")

	cAfter, err := store.GetLocationByID(ctx, orgID, c.ID)
	require.NoError(t, err)
	require.NotNil(t, cAfter)
	assert.Equal(t, "d.b.c", cAfter.Path, "C must follow B under its new parent")

	descendants, err := store.GetDescendants(ctx, orgID, d.ID)
	require.NoError(t, err)
	ids := make(map[int]bool)
	for _, loc := range descendants {
		ids[loc.ID] = true
	}
	assert.True(t, ids[b.ID], "GetDescendants(D) must include B")
	assert.True(t, ids[c.ID], "GetDescendants(D) must include C after cascade")
}

// TestUpdateLocation_ChangeExternalKey_CascadesDescendants verifies the
// cascade also fires on external_key rename, not just re-parent.
func TestUpdateLocation_ChangeExternalKey_CascadesDescendants(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	a := mkLoc(t, ctx, store, orgID, "a", nil)
	b := mkLoc(t, ctx, store, orgID, "b", &a.ID)
	c := mkLoc(t, ctx, store, orgID, "c", &b.ID)

	newKey := "b2"
	updated, err := store.UpdateLocation(ctx, orgID, b.ID, location.UpdateLocationRequest{
		ExternalKey: &newKey,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "a.b2", updated.Path)

	cAfter, err := store.GetLocationByID(ctx, orgID, c.ID)
	require.NoError(t, err)
	require.NotNil(t, cAfter)
	assert.Equal(t, "a.b2.c", cAfter.Path)
}

// TestRecomputeLocationPaths_Idempotent confirms the recompute function is a
// no-op on an already-canonical tree. Future migrations that touch path
// semantics can call it safely without corrupting good data.
func TestRecomputeLocationPaths_Idempotent(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	a := mkLoc(t, ctx, store, orgID, "wh-1", nil)
	mkLoc(t, ctx, store, orgID, "bay-3", &a.ID)

	var rows int
	err := pool.QueryRow(ctx, "SELECT trakrf.recompute_location_paths()").Scan(&rows)
	require.NoError(t, err)
	assert.Equal(t, 0, rows, "recompute on canonical tree must update zero rows")
}

// TestRecomputeLocationPaths_FixesLegacyRows simulates the BB15 D-2 finding:
// a row that bypassed the trigger and holds a non-canonical path. After
// recompute, the row matches the canonical rule and ancestor/descendant
// queries return correct results.
func TestRecomputeLocationPaths_FixesLegacyRows(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	parent := mkLoc(t, ctx, store, orgID, "WHS-01", nil)
	require.Equal(t, "whs_01", parent.Path)

	// Insert a child while bypassing the BEFORE trigger so we can plant a
	// deliberately non-canonical legacy path. Mirrors the preview-DB state
	// where 704/713 rows have stale paths.
	_, err := pool.Exec(ctx, "ALTER TABLE trakrf.locations DISABLE TRIGGER maintain_location_path")
	require.NoError(t, err)

	var childID int
	err = pool.QueryRow(ctx, `
		INSERT INTO trakrf.locations
		(name, external_key, parent_location_id, path, description, valid_from, is_active, org_id)
		VALUES ($1, $2, $3, $4::ltree, $5, $6, $7, $8)
		RETURNING id
	`, "WHS-07-03", "WHS-07-03", parent.ID, "WHS-01.WHS-07-03", "", time.Now(), true, orgID).Scan(&childID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, "ALTER TABLE trakrf.locations ENABLE TRIGGER maintain_location_path")
	require.NoError(t, err)

	// Sanity check: descendants query misses the legacy child (the bug).
	descBefore, err := store.GetDescendants(ctx, orgID, parent.ID)
	require.NoError(t, err)
	descIDsBefore := map[int]bool{}
	for _, l := range descBefore {
		descIDsBefore[l.ID] = true
	}
	assert.False(t, descIDsBefore[childID], "legacy non-canonical child should be invisible before recompute")

	// Run the recompute.
	var rows int
	err = pool.QueryRow(ctx, "SELECT trakrf.recompute_location_paths()").Scan(&rows)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, rows, 1, "at least the legacy child should be updated")

	// Child path is now canonical.
	child, err := store.GetLocationByID(ctx, orgID, childID)
	require.NoError(t, err)
	require.NotNil(t, child)
	assert.Equal(t, "whs_01.whs_07_03", child.Path)

	// Descendants query now finds the child.
	descAfter, err := store.GetDescendants(ctx, orgID, parent.ID)
	require.NoError(t, err)
	descIDsAfter := map[int]bool{}
	for _, l := range descAfter {
		descIDsAfter[l.ID] = true
	}
	assert.True(t, descIDsAfter[childID], "child must appear in descendants after recompute")

	// Ancestors query from the child finds the parent.
	ancestors, err := store.GetAncestors(ctx, orgID, childID)
	require.NoError(t, err)
	ancIDs := map[int]bool{}
	for _, l := range ancestors {
		ancIDs[l.ID] = true
	}
	assert.True(t, ancIDs[parent.ID], "parent must appear in ancestors after recompute")

	// Recompute is idempotent on the now-canonical state.
	err = pool.QueryRow(ctx, "SELECT trakrf.recompute_location_paths()").Scan(&rows)
	require.NoError(t, err)
	assert.Equal(t, 0, rows, "second recompute must be a no-op")
}
