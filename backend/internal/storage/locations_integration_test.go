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
