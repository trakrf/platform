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

func TestGetLocationByIdentifier_Found(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	parent, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-1", Name: "Warehouse 1", Path: "wh-1",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	_, err = store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-1.bay-3", Name: "Bay 3",
		ParentLocationID: &parent.ID,
		Path:             "wh-1.bay-3", ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	view, err := store.GetLocationByIdentifier(context.Background(), orgID, "wh-1.bay-3")
	require.NoError(t, err)
	require.NotNil(t, view)
	assert.Equal(t, "wh-1.bay-3", view.Identifier)
	require.NotNil(t, view.ParentIdentifier)
	assert.Equal(t, "wh-1", *view.ParentIdentifier)
}

func TestGetLocationByIdentifier_NotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	view, err := store.GetLocationByIdentifier(context.Background(), orgID, "missing")
	require.NoError(t, err)
	assert.Nil(t, view)
}

func TestListLocationsFiltered_Parent(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	root, _ := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "root", Name: "R", Path: "root",
		ValidFrom: time.Now(), IsActive: true,
	})
	_, _ = store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "root.a", Name: "A", ParentLocationID: &root.ID,
		Path: "root.a", ValidFrom: time.Now(), IsActive: true,
	})
	_, _ = store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "root.b", Name: "B", ParentLocationID: &root.ID,
		Path: "root.b", ValidFrom: time.Now(), IsActive: true,
	})

	items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
		ParentIdentifiers: []string{"root"},
		Sorts:             []location.ListSort{{Field: "identifier"}},
		Limit:             50,
	})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "root.a", items[0].Identifier)
	assert.Equal(t, "root.b", items[1].Identifier)
	require.NotNil(t, items[0].ParentIdentifier)
	assert.Equal(t, "root", *items[0].ParentIdentifier)
}

func TestListLocationsFiltered_Integration_IdentifiersNeverNil(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	// One location with no identifiers, one with a tag identifier.
	_, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "loc-empty", Name: "Empty", Path: "loc-empty",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	withTag, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "loc-tagged", Name: "Tagged", Path: "loc-tagged",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Insert a tag identifier for withTag. Table: trakrf.identifiers,
	// columns: org_id, type, value, location_id, valid_from, is_active.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.identifiers (org_id, type, value, location_id, valid_from, is_active)
		VALUES ($1, 'rfid', $2, $3, NOW(), true)
	`, orgID, "EPC-TAGGED", withTag.ID)
	require.NoError(t, err)

	items, err := store.ListLocationsFiltered(context.Background(), orgID, location.ListFilter{
		Sorts: []location.ListSort{{Field: "identifier"}},
		Limit: 50,
	})
	require.NoError(t, err)
	require.Len(t, items, 2)

	for _, item := range items {
		require.NotNil(t, item.Identifiers,
			"location %q Identifiers should not be nil (JSON would marshal to null)", item.Identifier)
	}

	var empty, tagged *location.LocationWithParent
	for i := range items {
		switch items[i].Identifier {
		case "loc-empty":
			empty = &items[i]
		case "loc-tagged":
			tagged = &items[i]
		}
	}
	require.NotNil(t, empty)
	require.NotNil(t, tagged)
	assert.Empty(t, empty.Identifiers, "loc-empty should have zero identifiers")
	assert.Len(t, tagged.Identifiers, 1)
	assert.Equal(t, "EPC-TAGGED", tagged.Identifiers[0].Value)
}
