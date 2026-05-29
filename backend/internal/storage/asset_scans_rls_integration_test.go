//go:build integration
// +build integration

// TRA-875: asset_scans was the only tenant table without RLS. This test is the
// analog of TRA-874's TestTestAppRole_RLSIsEnforced (which pins the locations
// policy): it proves the org-isolation policy on asset_scans is actually
// enforced for the non-superuser app role. Without it, a future asset_scans
// query that forgets WithOrgTx would silently return another org's scans — the
// silent sibling of the loud TRA-865 /history 500.

package storage_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestAssetScansRLS_IsEnforced(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()

	// Seed org A + an asset + one asset_scan via the admin (superuser) pool.
	// Superuser bypasses RLS, so these cross-org fixture writes need no org
	// context.
	orgA := testutil.CreateTestAccount(t, db.AdminPool)
	assetA := testutil.CreateTestAsset(t, db.AdminPool, orgA, "rls-scan-probe")

	scanTime := time.Now().UTC()
	_, err := db.AdminPool.Exec(ctx, `
		INSERT INTO trakrf.asset_scans (timestamp, org_id, asset_id)
		VALUES ($1, $2, $3)`, scanTime, orgA, assetA.ID)
	require.NoError(t, err, "admin pool must be able to seed across orgs")

	// 1. Read via the APP pool with NO org context. The policy is
	//    `org_id = current_setting('app.current_org_id')::BIGINT`; with the GUC
	//    unset the cast fails (42704/22P02). If this succeeds, the app pool is
	//    bypassing RLS and the policy gives no protection — the test must fail.
	var n int
	err = db.AppPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.asset_scans WHERE asset_id = $1`, assetA.ID).Scan(&n)
	require.Error(t, err,
		"app role must be RLS-enforced on asset_scans: a read without org "+
			"context must fail, but it succeeded — RLS is not protecting this table")

	// 2. The same read under org A's context returns the scan. This is what
	//    WithOrgTx does for every storage method.
	require.Equal(t, 1, countScansUnderOrg(t, db, orgA, assetA.ID),
		"scan must be visible to its owning org")

	// 3. Under a different org's context the scan is invisible — cross-org
	//    isolation. A missing WHERE org_id used to be the only thing standing
	//    between org B and org A's scans; now the policy is.
	orgB := createOrg(t, db.AdminPool, "Org B AssetScans RLS", "test-org-b-asset-scans-rls")
	require.Equal(t, 0, countScansUnderOrg(t, db, orgB, assetA.ID),
		"org B must not see org A's asset_scans")
}

// countScansUnderOrg runs the count inside a transaction that sets the org GUC,
// mirroring WithOrgTx, and returns how many of org A's scans are visible.
func countScansUnderOrg(t *testing.T, db *testutil.TestDB, orgID, assetID int) int {
	t.Helper()
	ctx := context.Background()

	tx, err := db.AppPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.current_org_id = %d", orgID))
	require.NoError(t, err)

	var n int
	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.asset_scans WHERE asset_id = $1`, assetID).Scan(&n)
	require.NoError(t, err, "read under a valid org context must not error")
	return n
}
