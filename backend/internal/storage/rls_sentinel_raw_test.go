//go:build integration
// +build integration

// Package storage_test: RLS sentinel-mode integration test.
//
// This file verifies two invariants that protect the platform against
// multi-tenant data leaks:
//
//  1. RAW-POOL SENTINEL: A database session with only `app.current_org_id = '0'`
//     set (the role-level default used in production as defense-in-depth) returns
//     zero rows on SELECT and fails with PgError code 42501 (RLS violation) on
//     INSERT, for all six RLS-protected tables.
//
//  2. WithOrgTx OVERRIDE: The same sentinel-configured pool, when wrapped by
//     Storage.WithOrgTx(ctx, realOrgID, fn), successfully reads and writes
//     because SET LOCAL inside the transaction overrides the session default.
//
// # Why the sentinel pool needs SET ROLE
//
// Integration tests connect as the `postgres` superuser, which carries the
// BYPASSRLS attribute and is exempt from all row-level security policies.
// To exercise RLS we must switch to a non-superuser role. This test creates
// a short-lived role (`rls_sentinel_test_role`) in the test DB and uses
// pgxpool.Config.AfterConnect to issue `SET ROLE` + `SET app.current_org_id`
// for every new connection.
//
// # Split-file decision
//
// This file (`rls_sentinel_raw_test.go`) covers only raw-pool and WithOrgTx-raw
// assertions that compile and run today with current method signatures.
// Storage-method-level assertions (calling GetAssetByID, GetIdentifiersByAssetID,
// etc. with the post-refactor orgID parameter) will be added in Task 8, after
// migration Tasks 3-7 update the method signatures. Adding those calls now
// would cause a compile failure that blocks the entire test binary.
package storage_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/storage"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// sentinelRoleName is the ephemeral non-superuser role created for this test.
// It must not conflict with roles used by other test packages running in
// parallel (see -p 1 serialization in justfile test-integration recipe).
const sentinelRoleName = "rls_sentinel_test_role"

// rlsTables is the full set of RLS-protected tables under trakrf schema.
var rlsTables = []string{
	"assets",
	"bulk_import_jobs",
	"identifiers",
	"locations",
	"scan_devices",
	"scan_points",
}

// setupSentinelRole creates a limited non-superuser role in the test database,
// grants it the minimum permissions needed to attempt reads and writes on the
// RLS-protected tables, and returns a cleanup function that revokes grants and
// drops the role.
//
// Precondition: pool must be connected as a superuser (postgres).
func setupSentinelRole(t *testing.T, pool *pgxpool.Pool) func() {
	t.Helper()
	ctx := context.Background()

	// Drop role if it was left over from a previous failed test run.
	_, _ = pool.Exec(ctx, fmt.Sprintf(`
		DO $$
		BEGIN
			IF EXISTS (SELECT FROM pg_roles WHERE rolname = '%s') THEN
				REVOKE ALL ON ALL TABLES    IN SCHEMA trakrf FROM %s;
				REVOKE ALL ON ALL SEQUENCES IN SCHEMA trakrf FROM %s;
				REVOKE USAGE ON SCHEMA trakrf FROM %s;
				DROP ROLE %s;
			END IF;
		END
		$$`, sentinelRoleName,
		sentinelRoleName, sentinelRoleName, sentinelRoleName, sentinelRoleName,
	))

	_, err := pool.Exec(ctx, fmt.Sprintf("CREATE ROLE %s NOINHERIT", sentinelRoleName))
	require.NoError(t, err, "create sentinel role")

	_, err = pool.Exec(ctx, fmt.Sprintf("GRANT USAGE ON SCHEMA trakrf TO %s", sentinelRoleName))
	require.NoError(t, err, "grant schema usage")

	_, err = pool.Exec(ctx, fmt.Sprintf(
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA trakrf TO %s",
		sentinelRoleName))
	require.NoError(t, err, "grant table permissions")

	_, err = pool.Exec(ctx, fmt.Sprintf(
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA trakrf TO %s",
		sentinelRoleName))
	require.NoError(t, err, "grant sequence permissions")

	return func() {
		revokeCtx := context.Background()
		_, _ = pool.Exec(revokeCtx, fmt.Sprintf(
			"REVOKE ALL ON ALL TABLES IN SCHEMA trakrf FROM %s", sentinelRoleName))
		_, _ = pool.Exec(revokeCtx, fmt.Sprintf(
			"REVOKE ALL ON ALL SEQUENCES IN SCHEMA trakrf FROM %s", sentinelRoleName))
		_, _ = pool.Exec(revokeCtx, fmt.Sprintf(
			"REVOKE USAGE ON SCHEMA trakrf FROM %s", sentinelRoleName))
		_, _ = pool.Exec(revokeCtx, fmt.Sprintf("DROP ROLE IF EXISTS %s", sentinelRoleName))
	}
}

// newSentinelPool creates a *pgxpool.Pool that mimics the production sentinel
// behavior: every connection switches to sentinelRoleName (a non-superuser
// that is subject to RLS) and sets app.current_org_id = '0' at session level.
//
// The returned pool is registered for t.Cleanup.
func newSentinelPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	dbURL := testutil.GetTestDatabaseURL()
	config, err := pgxpool.ParseConfig(dbURL)
	require.NoError(t, err, "parse test DB URL for sentinel pool")

	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		// Switch to the non-superuser role so RLS policies are enforced.
		if _, err := conn.Exec(ctx, fmt.Sprintf("SET ROLE %s", sentinelRoleName)); err != nil {
			return fmt.Errorf("SET ROLE %s: %w", sentinelRoleName, err)
		}
		// Mimic the production role-level default: app.current_org_id = '0'.
		if _, err := conn.Exec(ctx, "SET app.current_org_id = '0'"); err != nil {
			return fmt.Errorf("SET app.current_org_id: %w", err)
		}
		return nil
	}

	// Small pool — sentinel tests are not concurrent.
	config.MaxConns = 3
	config.MinConns = 1

	pool, err := pgxpool.NewWithConfig(ctx, config)
	require.NoError(t, err, "create sentinel pool")

	t.Cleanup(pool.Close)
	return pool
}

// assertReadEmpty checks that a raw pool SELECT COUNT(*) against the given
// trakrf table returns zero, demonstrating the sentinel blocks all reads.
func assertReadEmpty(t *testing.T, pool *pgxpool.Pool, table string) {
	t.Helper()
	ctx := context.Background()

	var count int
	err := pool.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM trakrf.%s", table)).Scan(&count)
	require.NoError(t, err, "SELECT COUNT(*) FROM trakrf.%s", table)
	assert.Equal(t, 0, count,
		"sentinel (current_org_id=0) must block all reads on trakrf.%s", table)
}

// assertInsertRejected checks that a raw INSERT into the given table fails
// with a 42501 (insufficient_privilege / RLS violation) PgError.
// The INSERT uses a real orgID so it is the RLS policy — not a FK or
// NOT NULL constraint — that rejects it.
func assertInsertRejected(t *testing.T, pool *pgxpool.Pool, table, insertSQL string, args ...interface{}) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, insertSQL, args...)
	require.Error(t, err, "INSERT into trakrf.%s should fail under sentinel", table)

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Errorf("trakrf.%s INSERT: expected *pgconn.PgError, got %T: %v", table, err, err)
		return
	}
	assert.Equal(t, "42501", pgErr.Code,
		"trakrf.%s INSERT: expected RLS violation code 42501, got %s (%s)",
		table, pgErr.Code, pgErr.Message)
}

// TestRLS_SentinelMode is the top-level test that drives the full RLS
// sentinel-mode verification. Run with:
//
//	just backend test-integration ./internal/storage/... -run TestRLS_SentinelMode
func TestRLS_SentinelMode(t *testing.T) {
	// ── 1. Bootstrap the standard test database (applies migrations). ──────────
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	superPool := store.Pool().(*pgxpool.Pool)

	// ── 2. Create the sentinel role and seed a real org for write tests. ───────
	dropRole := setupSentinelRole(t, superPool)
	defer dropRole()

	// Seed a real organization using the superuser pool (bypasses RLS).
	var realOrgID int
	err := superPool.QueryRow(context.Background(), `
		INSERT INTO trakrf.organizations (name, identifier, is_active)
		VALUES ('rls-sentinel-org', 'rls-sentinel-org', true)
		RETURNING id
	`).Scan(&realOrgID)
	require.NoError(t, err, "seed org for sentinel test")

	// ── 3. Build the sentinel pool (non-superuser, current_org_id = '0'). ─────
	sentinelPool := newSentinelPool(t)

	// ── 4. Raw-pool assertions ─────────────────────────────────────────────────
	//
	// For each RLS-protected table:
	//   (a) SELECT COUNT(*) returns 0 — sentinel blocks reads even if rows exist.
	//   (b) INSERT with the real org_id fails with 42501 — RLS blocks writes.
	//
	// We seed one row per table via the superuser pool first so the SELECT test
	// is meaningful (proves the row exists but is invisible under the sentinel).

	t.Run("raw_pool", func(t *testing.T) {
		// Seed data via superuser so each table has ≥1 row.
		seedTestData(t, superPool, realOrgID)

		t.Run("assets/read_empty", func(t *testing.T) {
			assertReadEmpty(t, sentinelPool, "assets")
		})

		t.Run("assets/insert_rejected", func(t *testing.T) {
			assertInsertRejected(t, sentinelPool, "assets",
				`INSERT INTO trakrf.assets
				 (org_id, identifier, name, type, valid_from, metadata, is_active, created_at, updated_at)
				 VALUES ($1, 'rls-sentinel-insert-asset', 'test', 'asset', $2, '{}', true, $3, $3)`,
				realOrgID, time.Now(), time.Now())
		})

		t.Run("bulk_import_jobs/read_empty", func(t *testing.T) {
			assertReadEmpty(t, sentinelPool, "bulk_import_jobs")
		})

		t.Run("bulk_import_jobs/insert_rejected", func(t *testing.T) {
			assertInsertRejected(t, sentinelPool, "bulk_import_jobs",
				`INSERT INTO trakrf.bulk_import_jobs (org_id, status, total_rows)
				 VALUES ($1, 'pending', 1)`,
				realOrgID)
		})

		t.Run("identifiers/read_empty", func(t *testing.T) {
			assertReadEmpty(t, sentinelPool, "identifiers")
		})

		t.Run("identifiers/insert_rejected", func(t *testing.T) {
			// Identifiers require an asset or location FK; seed asset ID was
			// created via seedTestData above — but the sentinel cannot see it.
			// The INSERT itself will be rejected by RLS before any FK check.
			assertInsertRejected(t, sentinelPool, "identifiers",
				`INSERT INTO trakrf.identifiers (org_id, type, value, valid_from, is_active)
				 VALUES ($1, 'epc', 'rls-sentinel-tag', $2, true)`,
				realOrgID, time.Now())
		})

		t.Run("locations/read_empty", func(t *testing.T) {
			assertReadEmpty(t, sentinelPool, "locations")
		})

		t.Run("locations/insert_rejected", func(t *testing.T) {
			assertInsertRejected(t, sentinelPool, "locations",
				`INSERT INTO trakrf.locations
				 (org_id, identifier, name, path, valid_from, is_active, created_at, updated_at)
				 VALUES ($1, 'rls-sentinel-loc', 'Sentinel Loc', 'rls-sentinel-loc', $2, true, $3, $3)`,
				realOrgID, time.Now(), time.Now())
		})

		t.Run("scan_devices/read_empty", func(t *testing.T) {
			assertReadEmpty(t, sentinelPool, "scan_devices")
		})

		t.Run("scan_devices/insert_rejected", func(t *testing.T) {
			assertInsertRejected(t, sentinelPool, "scan_devices",
				`INSERT INTO trakrf.scan_devices
				 (org_id, identifier, name, type, valid_from, is_active, created_at, updated_at)
				 VALUES ($1, 'rls-sentinel-dev', 'Sentinel Dev', 'reader', $2, true, $3, $3)`,
				realOrgID, time.Now(), time.Now())
		})

		t.Run("scan_points/read_empty", func(t *testing.T) {
			assertReadEmpty(t, sentinelPool, "scan_points")
		})

		t.Run("scan_points/insert_rejected", func(t *testing.T) {
			// scan_points requires a scan_device FK. We seeded one via seedTestData.
			// We can try inserting with a dummy scan_device_id — RLS fires before FK.
			assertInsertRejected(t, sentinelPool, "scan_points",
				`INSERT INTO trakrf.scan_points
				 (org_id, scan_device_id, identifier, name, valid_from, is_active, created_at, updated_at)
				 VALUES ($1, 99999, 'rls-sentinel-sp', 'Sentinel SP', $2, true, $3, $3)`,
				realOrgID, time.Now(), time.Now())
		})
	})

	// ── 5. WithOrgTx override assertions ──────────────────────────────────────
	//
	// These subtests verify that Storage.WithOrgTx correctly overrides the
	// sentinel (org_id = 0) by issuing SET LOCAL inside the transaction.
	//
	// Approach: use raw SQL inside the WithOrgTx closure (via pgx.Tx) to avoid
	// depending on post-refactor storage method signatures that don't exist yet.
	// Storage-method-level assertions (e.g. GetAssetByID with an orgID param)
	// will be added in Task 8, after Tasks 3-7 migrate the method signatures.

	t.Run("with_org_tx", func(t *testing.T) {
		sentinelStore := storage.NewWithPool(sentinelPool)

		// select_sees_rows: WithOrgTx must make realOrgID rows visible even
		// though the session has current_org_id = '0'.
		t.Run("select_sees_rows", func(t *testing.T) {
			var assetCount int
			err := sentinelStore.WithOrgTx(context.Background(), realOrgID,
				func(tx pgx.Tx) error {
					return tx.QueryRow(context.Background(),
						"SELECT COUNT(*) FROM trakrf.assets").Scan(&assetCount)
				},
			)
			require.NoError(t, err, "WithOrgTx SELECT must not error")
			assert.Greater(t, assetCount, 0,
				"WithOrgTx SET LOCAL must override sentinel: realOrgID assets must be visible")
		})

		// insert_succeeds: WithOrgTx must allow an INSERT with the real org_id
		// even though the raw pool would reject it with 42501.
		t.Run("insert_succeeds", func(t *testing.T) {
			err := sentinelStore.WithOrgTx(context.Background(), realOrgID,
				func(tx pgx.Tx) error {
					_, err := tx.Exec(context.Background(), `
						INSERT INTO trakrf.assets
						(org_id, identifier, name, type, valid_from, metadata, is_active, created_at, updated_at)
						VALUES ($1, 'rls-withorgtx-asset', 'WithOrgTx Asset', 'asset', $2, '{}', true, $3, $3)`,
						realOrgID, time.Now(), time.Now())
					return err
				},
			)
			require.NoError(t, err,
				"WithOrgTx INSERT with real org_id must succeed (SET LOCAL overrides sentinel)")
		})
	})
}

// seedTestData inserts one row into each of the six RLS-protected tables using
// the superuser pool (BYPASSRLS), so that assertReadEmpty can prove the sentinel
// hides existing rows, not just an empty table.
func seedTestData(t *testing.T, pool *pgxpool.Pool, orgID int) {
	t.Helper()
	ctx := context.Background()
	now := time.Now()

	// assets
	_, err := pool.Exec(ctx, `
		INSERT INTO trakrf.assets
		(org_id, identifier, name, type, valid_from, metadata, is_active, created_at, updated_at)
		VALUES ($1, 'rls-seed-asset', 'Seed Asset', 'asset', $2, '{}', true, $3, $3)
		ON CONFLICT DO NOTHING`,
		orgID, now, now)
	require.NoError(t, err, "seed asset row")

	// locations
	var locID int
	err = pool.QueryRow(ctx, `
		INSERT INTO trakrf.locations
		(org_id, identifier, name, path, valid_from, is_active, created_at, updated_at)
		VALUES ($1, 'rls-seed-loc', 'Seed Loc', 'rls-seed-loc', $2, true, $3, $3)
		ON CONFLICT DO NOTHING
		RETURNING id`,
		orgID, now, now).Scan(&locID)
	if err != nil {
		// ON CONFLICT DO NOTHING returns no row — fetch the existing one.
		err = pool.QueryRow(ctx,
			"SELECT id FROM trakrf.locations WHERE org_id = $1 AND identifier = 'rls-seed-loc'",
			orgID).Scan(&locID)
		require.NoError(t, err, "fetch existing seed location")
	}

	// scan_devices
	var devID int
	err = pool.QueryRow(ctx, `
		INSERT INTO trakrf.scan_devices
		(org_id, identifier, name, type, valid_from, is_active, created_at, updated_at)
		VALUES ($1, 'rls-seed-dev', 'Seed Dev', 'reader', $2, true, $3, $3)
		ON CONFLICT DO NOTHING
		RETURNING id`,
		orgID, now, now).Scan(&devID)
	if err != nil {
		err = pool.QueryRow(ctx,
			"SELECT id FROM trakrf.scan_devices WHERE org_id = $1 AND identifier = 'rls-seed-dev'",
			orgID).Scan(&devID)
		require.NoError(t, err, "fetch existing seed scan_device")
	}

	// scan_points
	_, err = pool.Exec(ctx, `
		INSERT INTO trakrf.scan_points
		(org_id, scan_device_id, identifier, name, valid_from, is_active, created_at, updated_at)
		VALUES ($1, $2, 'rls-seed-sp', 'Seed SP', $3, true, $4, $4)
		ON CONFLICT DO NOTHING`,
		orgID, devID, now, now)
	require.NoError(t, err, "seed scan_point row")

	// identifiers: the "identifier_target" check constraint requires exactly one
	// of asset_id or location_id to be non-null. Use the seeded location.
	_, err = pool.Exec(ctx, `
		INSERT INTO trakrf.identifiers
		(org_id, type, value, location_id, valid_from, is_active)
		VALUES ($1, 'epc', 'rls-seed-tag-value', $2, $3, true)
		ON CONFLICT DO NOTHING`,
		orgID, locID, now)
	require.NoError(t, err, "seed identifier row")

	// bulk_import_jobs
	_, err = pool.Exec(ctx, `
		INSERT INTO trakrf.bulk_import_jobs (org_id, status, total_rows)
		VALUES ($1, 'pending', 1)`,
		orgID)
	require.NoError(t, err, "seed bulk_import_job row")
}
