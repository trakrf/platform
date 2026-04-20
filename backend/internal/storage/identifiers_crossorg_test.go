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
	"github.com/trakrf/platform/backend/internal/models/shared"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestRemoveIdentifier_CrossOrgReturnsFalse(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgA := testutil.CreateTestAccount(t, pool)
	orgB := createOrg(t, pool, "Org B", "test-org-b")

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
		Value: "EPC-CROSS-ORG",
	})
	require.NoError(t, err)

	deleted, err := store.RemoveIdentifier(context.Background(), orgB, ident.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "cross-org identifier removal must return false")
}
