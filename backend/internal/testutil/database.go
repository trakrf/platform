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

func SetupTestDatabase(t *testing.T) *storage.Storage {
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

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("Failed to parse database URL: %v", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("Failed to create connection pool: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("Failed to ping database: %v", err)
	}

	store := storage.NewWithPool(pool)

	t.Cleanup(func() {
		cleanupTestData(t, pool)
		pool.Close()
	})

	return store
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

	t.Logf("✅ Created test database: trakrf_test")
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
		"trakrf.identifier_scans",
		"trakrf.identifiers",
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
