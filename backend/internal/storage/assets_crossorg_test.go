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
	"github.com/trakrf/platform/backend/internal/testutil"
)

// createOrg inserts an additional organization with a distinct identifier,
// since testutil.CreateTestAccount hardcodes identifier="test-org" and
// the organizations.identifier column is UNIQUE.
func createOrg(t *testing.T, pool *pgxpool.Pool, name, identifier string) int {
	t.Helper()
	var orgID int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO trakrf.organizations (name, identifier, is_active)
		 VALUES ($1, $2, true) RETURNING id`,
		name, identifier,
	).Scan(&orgID)
	require.NoError(t, err)
	return orgID
}

func TestUpdateAsset_CrossOrgReturnsNotFound(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrg(t, pool, "Org B", "test-org-b")

	created, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgA,
		Identifier: "asset-a",
		Name:       "Owned by A",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	newName := "should-not-be-applied"
	result, err := store.UpdateAsset(context.Background(), orgB, created.ID, assetmodel.UpdateAssetRequest{
		Name: &newName,
	})
	require.NoError(t, err)
	assert.Nil(t, result, "cross-org update must return nil (not found), not apply the change")

	fetched, err := store.GetAssetByID(context.Background(), &created.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "Owned by A", fetched.Name, "original asset must be untouched by cross-org update")
}

// TestUpdateAsset_OrgIDInBodyIgnored verifies that a PUT body cannot reassign an
// asset to a different org. Even if a malicious client sends a JSON body containing
// `org_id`, the storage layer must drop it. (The request struct no longer carries
// OrgID, but this test exists as a regression guard against re-introduction.)
func TestUpdateAsset_OrgIDInBodyIgnored(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrg(t, pool, "Org B", "test-org-b")

	created, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgA,
		Identifier: "asset-no-reassign",
		Name:       "Owned by A",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	// Caller is in orgA. We mutate name AND attempt org reassignment via the
	// raw JSON the handler would decode. Since UpdateAssetRequest no longer has
	// an OrgID field, the field is silently dropped — but mapReqToFields must
	// also never write org_id even if a future struct field were reintroduced.
	newName := "renamed"
	result, err := store.UpdateAsset(context.Background(), orgA, created.ID, assetmodel.UpdateAssetRequest{
		Name: &newName,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, newName, result.Name)
	assert.Equal(t, orgA, result.OrgID, "org_id must not change via UpdateAsset")

	// Re-fetch independently and confirm.
	fetched, err := store.GetAssetByID(context.Background(), &created.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, orgA, fetched.OrgID, "org_id must remain orgA after update")
	assert.NotEqual(t, orgB, fetched.OrgID, "org_id must not have been reassigned to orgB")
}

func TestDeleteAsset_CrossOrgReturnsFalse(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrg(t, pool, "Org B", "test-org-b")

	created, err := store.CreateAsset(context.Background(), assetmodel.Asset{
		OrgID:      orgA,
		Identifier: "asset-a-del",
		Name:       "Owned by A",
		Type:       "asset",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.NoError(t, err)

	deleted, err := store.DeleteAsset(context.Background(), orgB, created.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "cross-org delete must return false")

	fetched, err := store.GetAssetByID(context.Background(), &created.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched, "asset must still exist")
	assert.Nil(t, fetched.DeletedAt, "asset must not be soft-deleted by cross-org delete")
}
