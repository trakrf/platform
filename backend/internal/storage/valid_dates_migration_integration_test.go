//go:build integration
// +build integration

package storage_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

// TestNormalizeValidDatesMigration verifies the 000030 cleanup logic on
// representative bad rows. The test DB already has all migrations applied
// after SetupTestDB; we re-run the same UPDATE statements after seeding
// bad data directly to confirm the SQL logic works on future encounters too.
func TestNormalizeValidDatesMigration(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	defer testutil.CleanupAssets(t, pool)
	defer testutil.CleanupTestAccounts(t, pool)

	ctx := context.Background()
	orgID := testutil.CreateTestAccount(t, pool)

	// --- seed bad data directly (bypassing app layer) ---
	ts := time.Now().UnixNano()

	// Asset with both sentinels.
	var assetID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO trakrf.assets
			(name, identifier, type, description, org_id, valid_from, valid_to, is_active, metadata)
		VALUES
			($1, $2, 'asset', '', $3,
			 TIMESTAMPTZ '0001-01-01', TIMESTAMPTZ '2099-12-31',
			 true, '{}'::jsonb)
		RETURNING id
	`,
		fmt.Sprintf("tra468-asset-%d", ts),
		fmt.Sprintf("TRA468-ASSET-%d", ts),
		orgID,
	).Scan(&assetID)
	require.NoError(t, err)

	// Location with both sentinels.
	var locID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO trakrf.locations
			(org_id, identifier, name, is_active, valid_from, valid_to)
		VALUES
			($1, $2, $3, true,
			 TIMESTAMPTZ '0001-01-01', TIMESTAMPTZ '2099-12-31')
		RETURNING id
	`,
		orgID,
		fmt.Sprintf("TRA468-LOC-%d", ts),
		"tra468-loc",
	).Scan(&locID)
	require.NoError(t, err)

	// --- re-apply the migration's cleanup SQL ---
	cleanupStmts := []string{
		`UPDATE trakrf.organizations SET valid_from = created_at WHERE valid_from < TIMESTAMPTZ '1900-01-01'`,
		`UPDATE trakrf.organizations SET valid_to = NULL WHERE valid_to IS NOT NULL AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01')`,
		`UPDATE trakrf.assets SET valid_from = created_at WHERE valid_from < TIMESTAMPTZ '1900-01-01'`,
		`UPDATE trakrf.assets SET valid_to = NULL WHERE valid_to IS NOT NULL AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01')`,
		`UPDATE trakrf.locations SET valid_from = created_at WHERE valid_from < TIMESTAMPTZ '1900-01-01'`,
		`UPDATE trakrf.locations SET valid_to = NULL WHERE valid_to IS NOT NULL AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01')`,
		`UPDATE trakrf.identifiers SET valid_from = created_at WHERE valid_from < TIMESTAMPTZ '1900-01-01'`,
		`UPDATE trakrf.identifiers SET valid_to = NULL WHERE valid_to IS NOT NULL AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01')`,
	}
	for _, stmt := range cleanupStmts {
		_, err := pool.Exec(ctx, stmt)
		require.NoError(t, err, "stmt: %s", stmt)
	}

	// --- assert cleanup ---
	type row struct {
		validFrom time.Time
		validTo   *time.Time
		createdAt time.Time
	}

	check := func(t *testing.T, label, table string, id int64) {
		t.Helper()
		var r row
		err := pool.QueryRow(ctx,
			fmt.Sprintf(`SELECT valid_from, valid_to, created_at FROM trakrf.%s WHERE id = $1`, table),
			id).Scan(&r.validFrom, &r.validTo, &r.createdAt)
		require.NoError(t, err)
		require.True(t, r.validFrom.Equal(r.createdAt),
			"%s: valid_from = %v, want created_at %v", label, r.validFrom, r.createdAt)
		require.Nil(t, r.validTo, "%s: valid_to should be nil, got %v", label, r.validTo)
	}

	check(t, "asset", "assets", assetID)
	check(t, "location", "locations", locID)
}
