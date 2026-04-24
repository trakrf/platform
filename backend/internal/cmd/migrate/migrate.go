// Package migrate runs embedded database migrations as a one-shot command.
// It opens its own pgxpool using PG_URL, applies pending migrations via
// golang-migrate, logs the result, and returns. It does not start an HTTP
// server or any long-running goroutines.
package migrate

import (
	"context"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/trakrf/platform/backend/internal/buildinfo"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/migrations"
)

// Run applies all pending embedded migrations to the database identified
// by the PG_URL environment variable, then returns. A nil return means
// success (including the "no pending migrations" case).
func Run(ctx context.Context, info buildinfo.Info) error {
	log := logger.Get()

	pgURL := os.Getenv("PG_URL")
	if pgURL == "" {
		return fmt.Errorf("PG_URL environment variable not set")
	}

	config, err := pgxpool.ParseConfig(pgURL)
	if err != nil {
		return fmt.Errorf("failed to parse PG_URL: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer m.Close()

	log.Info().Str("version", info.Version).Str("commit", info.Commit).Msg("Starting migrations")

	err = m.Up()
	switch err {
	case nil:
		migrationVersion, dirty, _ := m.Version()
		log.Info().Uint("version", migrationVersion).Bool("dirty", dirty).Msg("Migrations complete")
		return nil
	case migrate.ErrNoChange:
		log.Info().Msg("No pending migrations")
		return nil
	default:
		return fmt.Errorf("migration failed: %w", err)
	}
}
