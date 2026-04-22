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

	locmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestUpdateLocation_CrossOrgReturnsNil(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrg(t, pool, "Org B Locations", "test-org-b-locations")

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgA,
		Identifier: "wh-a",
		Name:       "Owned by A",
		Path:       "wh-a",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	newName := "should-not-be-applied"
	result, err := store.UpdateLocation(context.Background(), orgB, created.ID, locmodel.UpdateLocationRequest{
		Name: &newName,
	})
	require.NoError(t, err)
	assert.Nil(t, result, "cross-org update must return nil (not found), not apply the change")

	fetched, err := store.GetLocationByID(context.Background(), orgA, created.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "Owned by A", fetched.Name, "original location must be untouched by cross-org update")
}

func TestDeleteLocation_CrossOrgReturnsFalse(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrg(t, pool, "Org B Locations Del", "test-org-b-locations-del")

	created, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID:      orgA,
		Identifier: "wh-a-del",
		Name:       "Owned by A",
		Path:       "wh-a-del",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	deleted, err := store.DeleteLocation(context.Background(), orgB, created.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "cross-org delete must return false")

	fetched, err := store.GetLocationByID(context.Background(), orgA, created.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "location must still exist")
	assert.Nil(t, fetched.DeletedAt, "location must not be soft-deleted by cross-org delete")
}
