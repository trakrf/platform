// Package migrate runs embedded database migrations as a one-shot command.
// It opens its own pgxpool using PG_URL, applies pending migrations via
// golang-migrate, logs the result, and returns. It does not start an HTTP
// server or any long-running goroutines.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/trakrf/platform/backend/internal/buildinfo"
	"github.com/trakrf/platform/backend/internal/logger"
	"github.com/trakrf/platform/backend/migrations"
)

// ledgerSchema pins the schema that holds golang-migrate's schema_migrations
// bookkeeping table (TRA-1069).
//
// Left unset, the postgres driver locates the ledger with CURRENT_SCHEMA().
// Every connection string in this project carries search_path=trakrf,public and
// migration 000001 creates the trakrf schema, so CURRENT_SCHEMA() returns
// "public" on a fresh database's first run and "trakrf" on every run after it.
// The ledger silently relocates, the new location starts empty at version 0,
// and the entire stack is replayed against an already-populated schema.
//
// "public" rather than "trakrf" because that is where the ledger already lives
// in every provisioned environment — preview and prod were both verified during
// TRA-1069 — and where a fresh database's first run puts it today. Pinning to
// trakrf would orphan those ledgers and replay migration 000001 on live data.
const ledgerSchema = "public"

// ddlSearchPath is the search_path this runner imposes on every connection it
// opens, and therefore the single point that decides where a migration's
// unqualified DDL lands (ADR 0003).
//
// Migrations are replayable artifacts. Letting ambient session state — a DSN
// parameter, a role default, an interactive session — decide their DDL target
// is how objects end up in the wrong schema silently, which is the same class of
// defect as TRA-1069. Setting it here means placement is a property of the
// runner: one authoritative value, in code, rather than a line repeated in every
// migration file and duplicated by whatever the caller's role happens to say.
//
// Files 000001-000038 also carry their own `SET search_path = trakrf, public`
// header. Those stay (they set the same value, and rewriting applied migrations
// buys nothing), but new migrations do not need one.
const ddlSearchPath = "trakrf, public"

// buildPoolConfig parses pgURL and imposes this runner's DDL search_path on
// every connection the pool opens, overriding whatever the DSN or the role
// default says. RuntimeParams is sent as a startup parameter, so it applies to
// each pooled connection rather than only the first.
func buildPoolConfig(pgURL string) (*pgxpool.Config, error) {
	config, err := pgxpool.ParseConfig(pgURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PG_URL: %w", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = ddlSearchPath
	return config, nil
}

// strayLedger is a schema_migrations table found outside ledgerSchema.
type strayLedger struct {
	schema  string
	version int64
	dirty   bool
}

// findStrayLedgers looks for schema_migrations tables outside ledgerSchema.
//
// Such a table is a second, divergent migration history left behind by the
// CURRENT_SCHEMA() drift described on ledgerSchema. The two ledgers record
// different versions, so the authoritative one reports a clean, fully-applied
// version that does not describe the schema actually on disk — the misleading
// success in TRA-1069, where trakrf.refresh_tokens was absent while the ledger
// claimed a clean version 38.
//
// pg_tables rather than information_schema.tables: the latter hides tables the
// current role has no privileges on, which would let a stray ledger owned by
// another role go unreported.
func findStrayLedgers(ctx context.Context, pool *pgxpool.Pool) ([]strayLedger, error) {
	rows, err := pool.Query(ctx, `
		SELECT schemaname
		FROM pg_tables
		WHERE tablename = $1 AND schemaname <> $2
		ORDER BY schemaname`, postgres.DefaultMigrationsTable, ledgerSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to check for stray migration ledgers: %w", err)
	}

	var schemas []string
	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to read stray migration ledger: %w", err)
		}
		schemas = append(schemas, schema)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to check for stray migration ledgers: %w", err)
	}

	strays := make([]strayLedger, 0, len(schemas))
	for _, schema := range schemas {
		stray := strayLedger{schema: schema}
		table := pgx.Identifier{schema, postgres.DefaultMigrationsTable}.Sanitize()
		// A ledger table with no row is a version-0 placeholder; report it as
		// version 0 rather than failing the whole preflight on ErrNoRows.
		err := pool.QueryRow(ctx,
			"SELECT version, dirty FROM "+table+" LIMIT 1").Scan(&stray.version, &stray.dirty)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("failed to read stray migration ledger %s: %w", table, err)
		}
		strays = append(strays, stray)
	}
	return strays, nil
}

// strayLedgerError renders an actionable message for a split migration history.
// Recovery is a human decision — reconcile the two histories or rebuild the
// schema — so this reports what was found rather than guessing.
func strayLedgerError(strays []strayLedger) error {
	var b strings.Builder
	fmt.Fprintf(&b, "refusing to migrate: found %d migration ledger(s) outside schema %q, "+
		"meaning this database has a split migration history (TRA-1069)",
		len(strays), ledgerSchema)
	for _, s := range strays {
		fmt.Fprintf(&b, "\n  %s.%s: version=%d dirty=%t",
			s.schema, postgres.DefaultMigrationsTable, s.version, s.dirty)
	}
	fmt.Fprintf(&b, "\n  %s.%s is authoritative. A stray ledger means some migrations were "+
		"recorded against a different history, so the version reported here does not describe "+
		"the schema on disk. Reconcile the two by hand, or rebuild the schema "+
		"(DROP SCHEMA trakrf CASCADE plus the stray ledger, then migrate); then drop the stray table.",
		ledgerSchema, postgres.DefaultMigrationsTable)
	return fmt.Errorf("%s", b.String())
}

// Run applies all pending embedded migrations to the database identified
// by the PG_URL environment variable, then returns. A nil return means
// success (including the "no pending migrations" case).
func Run(ctx context.Context, info buildinfo.Info) error {
	log := logger.Get()

	pgURL := os.Getenv("PG_URL")
	if pgURL == "" {
		return fmt.Errorf("PG_URL environment variable not set")
	}

	config, err := buildPoolConfig(pgURL)
	if err != nil {
		return err
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Preflight before golang-migrate touches anything: a split history must not
	// be papered over by creating or advancing a ledger.
	strays, err := findStrayLedgers(ctx, pool)
	if err != nil {
		return err
	}
	if len(strays) > 0 {
		return strayLedgerError(strays)
	}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to create migration source: %w", err)
	}

	driver, err := postgres.WithInstance(db, &postgres.Config{SchemaName: ledgerSchema})
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
