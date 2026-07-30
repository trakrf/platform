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

// appSchema owns everything this runner touches: the application objects and the
// migration ledger that tracks them.
const appSchema = "trakrf"

// ledgerSchema pins the schema holding golang-migrate's schema_migrations table
// (TRA-1069). Left unset, the postgres driver locates it with CURRENT_SCHEMA(),
// which on a fresh database resolves to public — trakrf does not exist until
// migration 000001 creates it — and to trakrf on every run afterwards. The ledger
// silently relocates, the new one starts at version 0, and the whole stack
// replays onto a populated schema.
//
// It lives in appSchema, not public, so that schema and its bookkeeping are one
// unit. That matters for the local/dev reset path: DROP SCHEMA trakrf CASCADE
// takes the ledger with it, leaving a genuinely empty database. A ledger in public
// would survive the drop still claiming version 38, and the next migrate would
// report "no pending migrations" against an empty schema — TRA-1069 reproduced by
// the reset procedure itself.
//
// That reset is for development databases only, and is deliberately not offered
// as recovery advice anywhere an operator might meet it mid-incident (TRA-1084).
const ledgerSchema = appSchema

// ddlSearchPath is imposed on every connection this runner opens, so a
// migration's unqualified DDL always resolves to the application schema no matter
// what the caller's DSN or role default says (ADR 0003).
//
// One line here means no migration has to declare it and no lint has to enforce
// that it did.
const ddlSearchPath = "trakrf, public"

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
		stray, err := readLedger(ctx, pool, schema)
		if err != nil {
			return nil, err
		}
		strays = append(strays, *stray)
	}
	return strays, nil
}

// readLedger reads version/dirty from the schema_migrations table in schema.
// The table is assumed to exist; callers that are not sure use findLedger.
func readLedger(ctx context.Context, pool *pgxpool.Pool, schema string) (*strayLedger, error) {
	ledger := strayLedger{schema: schema}
	table := pgx.Identifier{schema, postgres.DefaultMigrationsTable}.Sanitize()
	// A ledger table with no row is a version-0 placeholder; report it as
	// version 0 rather than failing the whole preflight on ErrNoRows.
	err := pool.QueryRow(ctx,
		"SELECT version, dirty FROM "+table+" LIMIT 1").Scan(&ledger.version, &ledger.dirty)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to read migration ledger %s: %w", table, err)
	}
	return &ledger, nil
}

// findLedger returns the ledger in schema, or nil when there is none.
//
// Whether the authoritative ledger exists is what separates "this database's
// history simply predates the pin and needs relocating" from "two divergent
// histories need reconciling" (TRA-1084), so the refusal message needs to know.
func findLedger(ctx context.Context, pool *pgxpool.Pool, schema string) (*strayLedger, error) {
	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_tables WHERE tablename = $1 AND schemaname = $2
		)`, postgres.DefaultMigrationsTable, schema).Scan(&exists); err != nil {
		return nil, fmt.Errorf("failed to check for the %s migration ledger: %w", schema, err)
	}
	if !exists {
		return nil, nil
	}
	return readLedger(ctx, pool, schema)
}

// strayLedgerError renders an actionable message for a ledger found outside
// ledgerSchema. Two quite different situations reach here and they do NOT share
// a remedy, so the message branches on whether the authoritative ledger exists
// (TRA-1084):
//
//   - Authoritative ledger absent. The stray is simply this database's real
//     history, predating the pin — every database created before TRA-1069 looks
//     like this, prod included. Relocating the table is the whole fix, so name
//     the exact statement.
//   - Both present. A genuinely split history, where neither version describes
//     the schema on disk on its own. That is a human judgement call about which
//     history to keep, so report both and stop.
//
// Deliberately suggests nothing destructive. This text is read by whoever is
// looking at a failed migrate Job, which on a live database is the worst
// possible moment to be handed a DROP. Failing safe — old pod still serving,
// database untouched — is the point of the preflight, and advice that undoes it
// on being followed would defeat it.
func strayLedgerError(strays []strayLedger, authoritative *strayLedger) error {
	var b strings.Builder
	fmt.Fprintf(&b, "refusing to migrate: found %d migration ledger(s) outside schema %q (TRA-1069)",
		len(strays), ledgerSchema)
	for _, s := range strays {
		fmt.Fprintf(&b, "\n  %s.%s: version=%d dirty=%t",
			s.schema, postgres.DefaultMigrationsTable, s.version, s.dirty)
	}

	if authoritative == nil {
		fmt.Fprintf(&b, "\n  %s.%s does not exist, so the ledger above is this database's real "+
			"migration history — it predates the pin to %q and only needs to be moved:",
			ledgerSchema, postgres.DefaultMigrationsTable, ledgerSchema)
		for _, s := range strays {
			fmt.Fprintf(&b, "\n      ALTER TABLE %s.%s SET SCHEMA %s;",
				s.schema, postgres.DefaultMigrationsTable, ledgerSchema)
		}
		fmt.Fprintf(&b, "\n  That is metadata-only, instant, and preserves both version and "+
			"dirty. Re-run migrate afterwards; it resumes from the version above.")
		return fmt.Errorf("%s", b.String())
	}

	fmt.Fprintf(&b, "\n  %s.%s: version=%d dirty=%t (authoritative)",
		ledgerSchema, postgres.DefaultMigrationsTable, authoritative.version, authoritative.dirty)
	fmt.Fprintf(&b, "\n  Both ledgers exist, so some migrations were recorded against a different "+
		"history and neither version on its own describes the schema on disk. Reconcile by hand: "+
		"compare both versions against the objects actually present, settle on one history in %q, "+
		"then drop the other table.", ledgerSchema)
	return fmt.Errorf("%s", b.String())
}

// Run applies all pending embedded migrations to the database identified by the
// PG_URL environment variable, then returns. A nil return means success
// (including the "no pending migrations" case).
func Run(ctx context.Context, info buildinfo.Info) error {
	pgURL := os.Getenv("PG_URL")
	if pgURL == "" {
		return fmt.Errorf("PG_URL environment variable not set")
	}
	return RunURL(ctx, pgURL, info)
}

// RunURL is Run against an explicit database URL. It exists so that everything
// which migrates goes through this one implementation rather than growing a
// second one: the integration harness used to shell out to the bare `migrate`
// CLI, which meant the schema bootstrap, the ledger pin and the split-history
// preflight all had to be duplicated there — or, as in TRA-1069, not be.
func RunURL(ctx context.Context, pgURL string, info buildinfo.Info) error {
	log := logger.Get()

	config, err := pgxpool.ParseConfig(pgURL)
	if err != nil {
		return fmt.Errorf("failed to parse PG_URL: %w", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = ddlSearchPath

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
		authoritative, err := findLedger(ctx, pool, ledgerSchema)
		if err != nil {
			return err
		}
		return strayLedgerError(strays, authoritative)
	}

	// Create the schema before golang-migrate looks for its ledger. This is the
	// one thing that cannot be left to a migration: the driver resolves the ledger
	// location, and creates the table, before migration 000001 runs. Without this
	// the ledger lands wherever CURRENT_SCHEMA() happens to point on a database
	// that does not have the schema yet — the root of TRA-1069. Idempotent, and
	// migration 000001 still declares it for a hand-applied run.
	if _, err := pool.Exec(ctx,
		"CREATE SCHEMA IF NOT EXISTS "+pgx.Identifier{appSchema}.Sanitize()); err != nil {
		return fmt.Errorf("failed to ensure schema %s exists: %w", appSchema, err)
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
