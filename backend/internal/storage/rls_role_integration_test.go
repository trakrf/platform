//go:build integration
// +build integration

// TRA-874: the integration test harness must connect storage to a
// non-superuser, RLS-enforced role. Postgres exempts superusers and table
// owners from row-level security, so a harness that runs storage as the
// `postgres` superuser cannot prove that a storage method sets org context
// (via WithOrgTx) before touching an RLS-protected table. A missing wrapper
// then ships green from CI — exactly how TRA-865's /history 500 reached
// preview.
//
// This is the keystone: it pins that the pool backing *storage.Storage is the
// RLS-enforced app role, while the fixture/cleanup pool stays superuser. If
// this passes, "missing WithOrgTx" fails an integration test, not just the
// check-rls-guard grep.

package storage_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/trakrf/platform/backend/internal/testutil"
)

func TestTestAppRole_RLSIsEnforced(t *testing.T) {
	db := testutil.SetupTestDBFull(t)
	ctx := context.Background()

	// Seed an org + a location via the admin (superuser) pool. Superuser
	// bypasses RLS, so this fixture write needs no org context — the
	// "escape hatch" production never gets.
	orgID := testutil.CreateTestAccount(t, db.AdminPool)
	var locID int
	err := db.AdminPool.QueryRow(ctx, `
		INSERT INTO trakrf.locations (org_id, external_key, name, valid_from, is_active)
		VALUES ($1, 'rls-probe', 'RLS Probe', now(), true)
		RETURNING id`, orgID).Scan(&locID)
	require.NoError(t, err, "admin pool must be able to seed across orgs")

	// 1. Read the same row via the APP pool with NO org context set. The RLS
	//    policy is `org_id = current_setting('app.current_org_id')::BIGINT`;
	//    with the GUC unset the cast fails (42704/22P02). If this read
	//    succeeds, the app pool is bypassing RLS (superuser or BYPASSRLS) and
	//    the harness gives no protection — the test must fail in that case.
	var n int
	err = db.AppPool.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.locations WHERE id = $1`, locID).Scan(&n)
	require.Error(t, err,
		"app role must be RLS-enforced: a read without org context must fail, "+
			"but it succeeded — the storage pool is bypassing RLS")

	// 2. The same read inside a transaction that sets org context returns the
	//    row. This is what WithOrgTx does for every storage method.
	tx, err := db.AppPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.current_org_id = %d", orgID))
	require.NoError(t, err)

	err = tx.QueryRow(ctx,
		`SELECT count(*) FROM trakrf.locations WHERE id = $1`, locID).Scan(&n)
	require.NoError(t, err, "read under correct org context must succeed")
	require.Equal(t, 1, n, "row must be visible to its owning org")
}
