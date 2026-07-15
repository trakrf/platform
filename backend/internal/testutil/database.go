package testutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/trakrf/platform/backend/internal/storage"
)

// testAppRole / testAppPassword identify the non-superuser, RLS-enforced role
// the integration harness runs storage methods as (TRA-874). It mirrors the
// production trakrf-app-<env> posture: CRUD on tables, USAGE/EXECUTE on the
// schema's sequences and functions, but no superuser, no BYPASSRLS, and no
// table ownership — so row-level security is actually evaluated against it.
const (
	testAppRole     = "trakrf_test_app"
	testAppPassword = "trakrf_test_app"
)

func GetPostgresURL() string {
	pgURL := os.Getenv("PG_URL")
	if pgURL != "" {
		pgURL = strings.Replace(pgURL, "timescaledb", "localhost", 1)
		pgURL = strings.Replace(pgURL, "/postgres?", "/postgres?", 1)
		return pgURL
	}
	return "postgresql://postgres:postgres@localhost:5432/postgres?sslmode=disable"
}

func GetTestDatabaseURL() string {
	testURL := os.Getenv("TEST_PG_URL")
	if testURL != "" {
		testURL = strings.Replace(testURL, "timescaledb", "localhost", 1)
		return testURL
	}

	pgURL := GetPostgresURL()
	return strings.Replace(pgURL, "/postgres?", "/trakrf_test?", 1)
}

// SetupTestDatabase returns a *storage.Storage whose methods run on the
// RLS-enforced app role. Storage's Pool() returns the superuser admin pool for
// fixture setup and cleanup. See SetupTestDBFull for the full harness.
func SetupTestDatabase(t *testing.T) *storage.Storage {
	t.Helper()
	return SetupTestDBFull(t).Store
}

func createTestDatabase(ctx context.Context, t *testing.T) error {
	t.Helper()

	pgURL := GetPostgresURL()

	conn, err := pgx.Connect(ctx, pgURL)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}
	defer conn.Close(ctx)

	_, err = conn.Exec(ctx, "DROP DATABASE IF EXISTS trakrf_test WITH (FORCE)")
	if err != nil {
		t.Logf("Warning: failed to drop test database: %v", err)
	}

	_, err = conn.Exec(ctx, "CREATE DATABASE trakrf_test")
	if err != nil {
		return fmt.Errorf("failed to create test database: %w", err)
	}

	// Set the database-level search_path so migrations and unqualified
	// trakrf-schema references resolve. (The id trigger itself now reads a
	// fully-qualified trakrf.id_seq, so it no longer depends on this.)
	_, err = conn.Exec(ctx, "ALTER DATABASE trakrf_test SET search_path TO trakrf, public")
	if err != nil {
		return fmt.Errorf("failed to set search_path on test database: %w", err)
	}

	// Set the obfuscation key required by trakrf.generate_obfuscated_id().
	// This is a fixed test key — production sets its own via ALTER DATABASE.
	_, err = conn.Exec(ctx, "ALTER DATABASE trakrf_test SET app.obfuscation_key = '6f626675736361746f72746573746b657920303132333435363738396162636465'")
	if err != nil {
		return fmt.Errorf("failed to set obfuscation_key on test database: %w", err)
	}

	// Create (or normalize) the non-superuser app role. CREATE ROLE is
	// cluster-level, so it survives the DROP/CREATE DATABASE above and persists
	// across test runs; the ALTER makes the attributes self-healing if a prior
	// run left the role in a different state.
	_, err = conn.Exec(ctx, fmt.Sprintf(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '%s') THEN
				CREATE ROLE %s LOGIN PASSWORD '%s';
			END IF;
		END
		$$;`, testAppRole, testAppRole, testAppPassword))
	if err != nil {
		return fmt.Errorf("failed to create test app role: %w", err)
	}

	_, err = conn.Exec(ctx, fmt.Sprintf(
		"ALTER ROLE %s WITH LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE PASSWORD '%s'",
		testAppRole, testAppPassword))
	if err != nil {
		return fmt.Errorf("failed to normalize test app role: %w", err)
	}

	t.Logf("✅ Created test database: trakrf_test")
	return nil
}

// grantTestAppRole gives the non-superuser app role the same posture as the
// production trakrf-app-<env> role: CRUD on tables, USAGE/SELECT on sequences,
// EXECUTE on functions, USAGE on the schemas. It deliberately grants no
// TRUNCATE and no ownership, so RLS is enforced for this role. Run after
// migrations (objects must exist) and re-run every test since the database is
// recreated each time.
func grantTestAppRole(ctx context.Context, t *testing.T, dbURL string) error {
	t.Helper()

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to grant app role: %w", err)
	}
	defer conn.Close(ctx)

	stmts := []string{
		fmt.Sprintf("GRANT CONNECT ON DATABASE trakrf_test TO %s", testAppRole),
		fmt.Sprintf("GRANT USAGE ON SCHEMA trakrf TO %s", testAppRole),
		fmt.Sprintf("GRANT USAGE ON SCHEMA public TO %s", testAppRole),
		fmt.Sprintf("GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA trakrf TO %s", testAppRole),
		fmt.Sprintf("GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA trakrf TO %s", testAppRole),
		fmt.Sprintf("GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA trakrf TO %s", testAppRole),
	}
	for _, stmt := range stmts {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("failed to grant to test app role (%q): %w", stmt, err)
		}
	}

	t.Logf("✅ Granted RLS-enforced posture to role: %s", testAppRole)
	return nil
}

func getMigrationsPath(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	possiblePaths := []string{
		filepath.Join(wd, "..", "database", "migrations"),
		filepath.Join(wd, "..", "..", "database", "migrations"),
		filepath.Join(wd, "..", "..", "..", "database", "migrations"),
		filepath.Join(wd, "..", "..", "..", "..", "database", "migrations"),
		// backend/migrations layout (no database/ subdirectory)
		filepath.Join(wd, "..", "migrations"),
		filepath.Join(wd, "..", "..", "migrations"),
		filepath.Join(wd, "..", "..", "..", "migrations"),
		filepath.Join(wd, "..", "..", "..", "..", "migrations"),
	}

	for _, path := range possiblePaths {
		cleanPath := filepath.Clean(path)
		absPath, err := filepath.Abs(cleanPath)
		if err != nil {
			continue
		}
		if stat, err := os.Stat(absPath); err == nil && stat.IsDir() {
			entries, err := os.ReadDir(absPath)
			if err == nil && len(entries) > 0 {
				t.Logf("✅ Found migrations at: %s (%d files)", absPath, len(entries))
				return absPath
			}
		}
	}

	t.Fatalf("Migrations directory not found or empty. Tried: %v", possiblePaths)
	return ""
}

func runMigrations(dbURL, migrationsPath string, t *testing.T) error {
	t.Helper()

	migrateBinary := findMigrateBinary()
	if migrateBinary == "" {
		return fmt.Errorf("migrate binary not found in PATH - install with: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest")
	}

	cmd := exec.Command(migrateBinary,
		"-path", migrationsPath,
		"-database", dbURL,
		"up",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "no change") {
			t.Logf("✅ Migrations already up to date")
			return nil
		}
		return fmt.Errorf("migrate command failed: %w\nOutput: %s", err, string(output))
	}

	t.Logf("✅ Migrations applied successfully")
	return nil
}

func findMigrateBinary() string {
	// Check PATH first
	for _, name := range []string{"migrate", "golang-migrate"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}

	// Check common Go binary locations
	homeDir, err := os.UserHomeDir()
	if err == nil {
		goBinPath := filepath.Join(homeDir, "go", "bin", "migrate")
		if _, err := os.Stat(goBinPath); err == nil {
			return goBinPath
		}
	}

	return ""
}

func cleanupTestData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()

	tables := []string{
		"trakrf.bulk_import_jobs",
		"trakrf.asset_scans",
		"trakrf.tag_scans",
		"trakrf.tags",
		"trakrf.assets",
		"trakrf.scan_points",
		"trakrf.scan_devices",
		"trakrf.locations",
		"trakrf.org_users",
		"trakrf.users",
		"trakrf.organizations",
	}

	for _, table := range tables {
		_, err := pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", table))
		if err != nil {
			t.Logf("Warning: failed to truncate %s: %v", table, err)
		}
	}
}

// TestDB bundles a configured *storage.Storage with direct handles to both
// connection pools used by the integration test harness (TRA-874).
type TestDB struct {
	// Store runs every storage method on the RLS-enforced app pool.
	Store *storage.Storage
	// AdminPool is the superuser pool: cross-org fixture setup and cleanup.
	AdminPool *pgxpool.Pool
	// AppPool is the non-superuser, RLS-enforced pool backing Store.
	AppPool *pgxpool.Pool
}

// SetupTestDBFull creates the test database, runs migrations, and returns a
// TestDB with both the superuser admin pool and the RLS-enforced app pool.
// SetupTestDB is the thin back-compat wrapper used by most tests.
func SetupTestDBFull(t *testing.T) *TestDB {
	t.Helper()

	ctx := context.Background()

	if err := createTestDatabase(ctx, t); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	dbURL := GetTestDatabaseURL()

	migrationsPath := getMigrationsPath(t)
	if err := runMigrations(dbURL, migrationsPath, t); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	if err := grantTestAppRole(ctx, t, dbURL); err != nil {
		t.Fatalf("Failed to grant test app role: %v", err)
	}

	adminPool := openPool(ctx, t, dbURL, nil)
	// The app pool connects as the non-superuser, RLS-enforced role. Storage
	// methods run here, so a missing WithOrgTx fails the test.
	appPool := openPool(ctx, t, dbURL, func(c *pgxpool.Config) {
		c.ConnConfig.User = testAppRole
		c.ConnConfig.Password = testAppPassword
	})

	store := storage.NewForTest(appPool, adminPool)

	t.Cleanup(func() {
		cleanupTestData(t, adminPool)
		appPool.Close()
		adminPool.Close()
	})

	return &TestDB{Store: store, AdminPool: adminPool, AppPool: appPool}
}

func openPool(ctx context.Context, t *testing.T, url string, mutate func(*pgxpool.Config)) *pgxpool.Pool {
	t.Helper()

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("Failed to parse database URL: %v", err)
	}

	if mutate != nil {
		mutate(config)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create connection pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("Failed to ping database: %v", err)
	}

	return pool
}

// SetupTestDB sets up a test database and returns storage with cleanup function.
// This is the preferred method for integration tests.
func SetupTestDB(t *testing.T) (*storage.Storage, func()) {
	t.Helper()
	store := SetupTestDatabase(t)

	cleanup := func() {
		if pool, ok := store.Pool().(*pgxpool.Pool); ok {
			cleanupTestData(t, pool)
		}
	}

	return store, cleanup
}

// CleanupAssets truncates the assets table.
func CleanupAssets(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, "TRUNCATE TABLE trakrf.assets RESTART IDENTITY CASCADE")
	if err != nil {
		t.Logf("Warning: failed to truncate assets: %v", err)
	}
}

// CleanupTestAccounts truncates the organizations table.
func CleanupTestAccounts(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, "TRUNCATE TABLE trakrf.organizations RESTART IDENTITY CASCADE")
	if err != nil {
		t.Logf("Warning: failed to truncate organizations: %v", err)
	}
}

// CreateTestAccount creates a test organization and returns its ID.
func CreateTestAccount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	ctx := context.Background()

	var accountID int
	err := pool.QueryRow(ctx, `
		INSERT INTO trakrf.organizations (name, identifier, is_active)
		VALUES ($1, $2, $3)
		RETURNING id
	`, "Test Organization", "test-org", true).Scan(&accountID)

	if err != nil {
		t.Fatalf("Failed to create test account: %v", err)
	}

	t.Logf("✅ Created test account: %d", accountID)
	return accountID
}

// RefreshAssetScanLatest fully refreshes the asset_scan_latest continuous
// aggregate (TRA-1022) so it reflects rows seeded since the last refresh.
//
// The CAGG is materialized-only (real-time aggregation is incompatible with the
// RLS on asset_scans), so a read sees only materialized data. In production a
// background policy refreshes every ~30s; tests seed and query in the same
// instant, so they must force a refresh in between or the report comes back
// empty. Call this after seeding asset_scans and before hitting the endpoint.
//
// refresh_continuous_aggregate manages its own transaction and cannot run
// inside one, so it goes through the simple protocol — pgx would otherwise wrap
// a parameterless Exec in the extended protocol's implicit transaction. NULL,
// NULL refreshes the whole range, including the current (incomplete) bucket that
// the policy's end_offset would normally leave to real-time aggregation.
func RefreshAssetScanLatest(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"CALL refresh_continuous_aggregate('trakrf.asset_scan_latest', NULL, NULL)",
		pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		t.Fatalf("failed to refresh asset_scan_latest CAGG: %v", err)
	}
}
