package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
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

	migrationsPath := getMigrationsPath(t)
	if err := runMigrations(dbURL, migrationsPath); err != nil {
		pool.Close()
		t.Fatalf("Failed to run migrations: %v", err)
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

func runMigrations(dbURL, migrationsPath string) error {
	m, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsPath),
		dbURL,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
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
