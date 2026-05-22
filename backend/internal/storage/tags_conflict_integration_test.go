//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func rfidReq(value string) shared.TagRequest {
	t := "rfid"
	return shared.TagRequest{TagType: &t, Value: value}
}

// seedLocation inserts a location directly, mirroring testutil.CreateTestAsset.
func seedLocation(t *testing.T, pool *pgxpool.Pool, orgID int, externalKey, name string) int {
	t.Helper()
	now := time.Now()
	var id int
	err := pool.QueryRow(context.Background(), `
		INSERT INTO trakrf.locations (org_id, external_key, name, valid_from, valid_to, is_active)
		VALUES ($1, $2, $3, $4, $5, TRUE)
		RETURNING id
	`, orgID, externalKey, name, now, now.Add(24*time.Hour)).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestAddTag_CrossAssetConflict(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	assetA := testutil.CreateTestAsset(t, pool, orgID, "AST-A")
	assetB := testutil.CreateTestAsset(t, pool, orgID, "AST-B")

	value := "E2000000CONFLICT01"
	_, err := store.AddTagToAsset(ctx, orgID, assetA.ID, rfidReq(value))
	require.NoError(t, err)

	_, err = store.AddTagToAsset(ctx, orgID, assetB.ID, rfidReq(value))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists", "handler keys its 409 on this substring")
	assert.Contains(t, err.Error(), "asset")
	assert.Contains(t, err.Error(), "AST-A", "names the conflicting asset's external_key")
}

func TestAddTag_CrossLocationConflict(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	locID := seedLocation(t, pool, orgID, "LOC-DOCK3", "Dock 3")
	assetB := testutil.CreateTestAsset(t, pool, orgID, "AST-B")

	value := "E2000000CONFLICT02"
	_, err := store.AddTagToLocation(ctx, orgID, locID, rfidReq(value))
	require.NoError(t, err)

	_, err = store.AddTagToAsset(ctx, orgID, assetB.ID, rfidReq(value))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.Contains(t, err.Error(), "location")
	assert.Contains(t, err.Error(), "Dock 3", "names the conflicting location")
	assert.Contains(t, err.Error(), "LOC-DOCK3")
}

func TestAddTag_SoftDeletedRowNotBlocking(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	ctx := context.Background()

	orgID := testutil.CreateTestAccount(t, pool)
	assetA := testutil.CreateTestAsset(t, pool, orgID, "AST-A")
	assetB := testutil.CreateTestAsset(t, pool, orgID, "AST-B")

	value := "E2000000CONFLICT03"
	tag, err := store.AddTagToAsset(ctx, orgID, assetA.ID, rfidReq(value))
	require.NoError(t, err)

	removed, err := store.RemoveAssetTag(ctx, orgID, assetA.ID, tag.ID)
	require.NoError(t, err)
	require.True(t, removed)

	// The soft-deleted row must not block re-using the value elsewhere.
	_, err = store.AddTagToAsset(ctx, orgID, assetB.ID, rfidReq(value))
	require.NoError(t, err)
}
