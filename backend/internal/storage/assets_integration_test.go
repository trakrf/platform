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

	_, err = store.DeleteAsset(context.Background(), &created.ID)
	require.NoError(t, err)

	view, err := store.GetAssetByIdentifier(context.Background(), orgID, "gone")
	require.NoError(t, err)
	assert.Nil(t, view)
}
