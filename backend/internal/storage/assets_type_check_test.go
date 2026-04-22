//go:build integration
// +build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/models/asset"
	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestAssets_TypeCheck_AcceptsPersonAndInventory(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	for _, kind := range []string{"asset", "person", "inventory"} {
		kind := kind
		t.Run(kind, func(t *testing.T) {
			a, err := store.CreateAsset(context.Background(), asset.Asset{
				OrgID:      orgID,
				Identifier: "tra447-" + kind,
				Name:       kind + " record",
				Type:       kind,
				ValidFrom:  time.Now(),
				IsActive:   true,
			})
			require.NoError(t, err)
			require.NotNil(t, a)
			assert.Equal(t, kind, a.Type)
		})
	}
}

func TestAssets_TypeCheck_RejectsUnknown(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	_, err := store.CreateAsset(context.Background(), asset.Asset{
		OrgID:      orgID,
		Identifier: "tra447-widget",
		Name:       "widget",
		Type:       "widget",
		ValidFrom:  time.Now(),
		IsActive:   true,
	})
	require.Error(t, err, "widget must violate the type CHECK constraint")
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "error must unwrap to a pgconn.PgError")
	assert.Equal(t, "23514", pgErr.Code, "must be a check_violation (SQLSTATE 23514)")
}
