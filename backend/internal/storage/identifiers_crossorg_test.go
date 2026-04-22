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

	assetmodel "github.com/trakrf/platform/backend/internal/models/asset"
	locationmodel "github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// TestRemoveAssetIdentifier_CrossOrg_ReturnsFalse verifies that an API-key
// caller in orgB cannot delete an identifier owned by an asset in orgA.
// It also asserts the identifier row survives (deleted_at still NULL) to
// guard against the storage layer mutating state before short-circuiting.
func TestRemoveAssetIdentifier_CrossOrg_ReturnsFalse(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrg(t, pool, "Org B", "test-org-b-asset-ident")

	created, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgA,
		Identifier: "ident-host-a",
		Name:       "A",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToAsset(context.Background(), orgA, created.ID, shared.TagIdentifierRequest{
		Type:  "epc",
		Value: "EPC-CROSS-ORG-ASSET",
	})
	require.NoError(t, err)

	// orgB attempts to delete orgA's identifier.
	deleted, err := store.RemoveAssetIdentifier(context.Background(), orgB, created.ID, ident.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "cross-org asset identifier removal must return false")

	// Confirm the identifier was NOT mutated.
	fetched, err := store.GetIdentifierByID(context.Background(), orgA, ident.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "identifier must still exist after cross-org removal attempt")
	assert.Equal(t, ident.ID, fetched.ID)
}

// TestRemoveAssetIdentifier_WrongAssetID_ReturnsFalse verifies that the
// assetID path param is load-bearing: an identifier belonging to one asset
// cannot be deleted by referencing a different asset of the same org.
func TestRemoveAssetIdentifier_WrongAssetID_ReturnsFalse(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)

	assetOwner, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgA,
		Identifier: "ident-owner",
		Name:       "Owner",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	assetBystander, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgA,
		Identifier: "ident-bystander",
		Name:       "Bystander",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToAsset(context.Background(), orgA, assetOwner.ID, shared.TagIdentifierRequest{
		Type:  "epc",
		Value: "EPC-OWNER",
	})
	require.NoError(t, err)

	// Path claims bystander asset, but identifier actually belongs to owner.
	deleted, err := store.RemoveAssetIdentifier(context.Background(), orgA, assetBystander.ID, ident.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "removal via wrong assetID must return false")

	// Identifier must still exist.
	fetched, err := store.GetIdentifierByID(context.Background(), orgA, ident.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "identifier must still exist after wrong-assetID removal attempt")
}

// TestRemoveLocationIdentifier_CrossOrg_ReturnsFalse: same pattern as asset
// cross-org, but for location-scoped identifiers.
func TestRemoveLocationIdentifier_CrossOrg_ReturnsFalse(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrg(t, pool, "Org B", "test-org-b-loc-ident")

	loc, err := store.CreateLocation(context.Background(), locationmodel.Location{
		OrgID:      orgA,
		Identifier: "LOC-HOST-A",
		Name:       "Loc A",
		Path:       "LOC-HOST-A",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToLocation(context.Background(), orgA, loc.ID, shared.TagIdentifierRequest{
		Type:  "epc",
		Value: "EPC-CROSS-ORG-LOC",
	})
	require.NoError(t, err)

	deleted, err := store.RemoveLocationIdentifier(context.Background(), orgB, loc.ID, ident.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "cross-org location identifier removal must return false")

	fetched, err := store.GetIdentifierByID(context.Background(), orgA, ident.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "identifier must still exist after cross-org removal attempt")
	assert.Equal(t, ident.ID, fetched.ID)
}

// TestRemoveLocationIdentifier_WrongLocationID_ReturnsFalse: same pattern as
// wrong-assetID, but for location-scoped identifiers.
func TestRemoveLocationIdentifier_WrongLocationID_ReturnsFalse(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)

	locOwner, err := store.CreateLocation(context.Background(), locationmodel.Location{
		OrgID:      orgA,
		Identifier: "LOC-OWNER",
		Name:       "Owner",
		Path:       "LOC-OWNER",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	locBystander, err := store.CreateLocation(context.Background(), locationmodel.Location{
		OrgID:      orgA,
		Identifier: "LOC-BYSTANDER",
		Name:       "Bystander",
		Path:       "LOC-BYSTANDER",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	ident, err := store.AddIdentifierToLocation(context.Background(), orgA, locOwner.ID, shared.TagIdentifierRequest{
		Type:  "epc",
		Value: "EPC-LOC-OWNER",
	})
	require.NoError(t, err)

	deleted, err := store.RemoveLocationIdentifier(context.Background(), orgA, locBystander.ID, ident.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "removal via wrong locationID must return false")

	fetched, err := store.GetIdentifierByID(context.Background(), orgA, ident.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "identifier must still exist after wrong-locationID removal attempt")
}
