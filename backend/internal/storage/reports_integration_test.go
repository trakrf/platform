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
	"github.com/trakrf/platform/backend/internal/models/report"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestCurrentLocations_QMatchesActiveIdentifierOnly(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-current", Name: "Current WH", Path: "wh-current",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	activeAsset, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "asset-current-active", Name: "ActiveCur",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	deletedIDAsset, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "asset-current-deleted", Name: "DeletedCur",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Insert asset_scans so both assets appear in the ListCurrentLocations query
	// (which uses asset_scans, not current_location_id on assets).
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id)
		VALUES (NOW(), $1, $2, $3)
	`, orgID, activeAsset.ID, loc.ID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id)
		VALUES (NOW(), $1, $2, $3)
	`, orgID, deletedIDAsset.ID, loc.ID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.tags (org_id, type, value, asset_id, valid_from, is_active)
		VALUES ($1, 'rfid', 'CUR-ACTIVE-30077', $2, NOW(), true)
	`, orgID, activeAsset.ID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.tags (org_id, type, value, asset_id, valid_from, is_active, deleted_at)
		VALUES ($1, 'rfid', 'CUR-DELETED-30077', $2, NOW(), true, NOW())
	`, orgID, deletedIDAsset.ID)
	require.NoError(t, err)

	t.Run("active identifier matches", func(t *testing.T) {
		q := "ACTIVE-30077"
		items, err := store.ListCurrentLocations(context.Background(), orgID, report.CurrentLocationFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "asset-current-active", items[0].AssetIdentifier)
	})

	t.Run("soft-deleted identifier does not match", func(t *testing.T) {
		q := "DELETED-30077"
		items, err := store.ListCurrentLocations(context.Background(), orgID, report.CurrentLocationFilter{
			Q: &q, Limit: 50,
		})
		require.NoError(t, err)
		assert.Empty(t, items)
	})
}

func TestCurrentLocations_CountExcludesSoftDeletedIdentifier(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-count", Name: "Count WH", Path: "wh-count",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	deletedIDAsset, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "asset-count-deleted", Name: "DeletedCount",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Insert asset_scan so the asset appears in the count query.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id)
		VALUES (NOW(), $1, $2, $3)
	`, orgID, deletedIDAsset.ID, loc.ID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.tags (org_id, type, value, asset_id, valid_from, is_active, deleted_at)
		VALUES ($1, 'rfid', 'COUNT-DELETED-40099', $2, NOW(), true, NOW())
	`, orgID, deletedIDAsset.ID)
	require.NoError(t, err)

	q := "COUNT-DELETED-40099"
	count, err := store.CountCurrentLocations(context.Background(), orgID, report.CurrentLocationFilter{
		Q: &q, Limit: 50,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestCurrentLocations_DefaultElidesDeletedAsset(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	loc, err := store.CreateLocation(context.Background(), location.Location{
		OrgID: orgID, Identifier: "wh-elide", Name: "Elide WH", Path: "wh-elide",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	liveAsset, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "asset-live-elide", Name: "LiveElide",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	deletedAsset, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID: orgID, Identifier: "asset-deleted-elide", Name: "DeletedElide",
		CurrentLocationID: &loc.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	// Both assets have scans so both would appear without elision.
	_, err = pool.Exec(context.Background(), `
		INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id, location_id)
		VALUES (NOW(), $1, $2, $3), (NOW(), $1, $4, $3)
	`, orgID, liveAsset.ID, loc.ID, deletedAsset.ID)
	require.NoError(t, err)

	// Soft-delete the second asset.
	_, err = pool.Exec(context.Background(), `
		UPDATE trakrf.assets SET deleted_at = NOW() WHERE id = $1
	`, deletedAsset.ID)
	require.NoError(t, err)

	t.Run("default elides deleted asset", func(t *testing.T) {
		items, err := store.ListCurrentLocations(context.Background(), orgID, report.CurrentLocationFilter{
			Limit: 50,
		})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "asset-live-elide", items[0].AssetIdentifier)
		assert.Nil(t, items[0].AssetDeletedAt)

		count, err := store.CountCurrentLocations(context.Background(), orgID, report.CurrentLocationFilter{
			Limit: 50,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("include_deleted=true returns deleted asset with timestamp", func(t *testing.T) {
		items, err := store.ListCurrentLocations(context.Background(), orgID, report.CurrentLocationFilter{
			IncludeDeleted: true,
			Limit:          50,
		})
		require.NoError(t, err)
		require.Len(t, items, 2)

		byIdent := map[string]report.CurrentLocationItem{}
		for _, it := range items {
			byIdent[it.AssetIdentifier] = it
		}
		assert.Nil(t, byIdent["asset-live-elide"].AssetDeletedAt)
		require.NotNil(t, byIdent["asset-deleted-elide"].AssetDeletedAt)
		assert.WithinDuration(t, time.Now(), *byIdent["asset-deleted-elide"].AssetDeletedAt, time.Minute)

		count, err := store.CountCurrentLocations(context.Background(), orgID, report.CurrentLocationFilter{
			IncludeDeleted: true,
			Limit:          50,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("q-search default skips deleted asset by name", func(t *testing.T) {
		q := "DeletedElide"
		items, err := store.ListCurrentLocations(context.Background(), orgID, report.CurrentLocationFilter{
			Q:     &q,
			Limit: 50,
		})
		require.NoError(t, err)
		assert.Empty(t, items, "default q-search must not surface deleted assets")
	})

	t.Run("q-search with include_deleted=true surfaces deleted asset by name", func(t *testing.T) {
		q := "DeletedElide"
		items, err := store.ListCurrentLocations(context.Background(), orgID, report.CurrentLocationFilter{
			Q:              &q,
			IncludeDeleted: true,
			Limit:          50,
		})
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "asset-deleted-elide", items[0].AssetIdentifier)
		require.NotNil(t, items[0].AssetDeletedAt)
	})
}
