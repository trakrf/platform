//go:build integration
// +build integration

// TRA-1069 — the migration ledger must live in a fixed schema.
//
// golang-migrate's postgres driver locates schema_migrations via
// CURRENT_SCHEMA() when Config.SchemaName is empty. Our connection strings
// carry search_path=trakrf,public, and on a fresh database the trakrf schema
// does not exist yet — so CURRENT_SCHEMA() resolves to public on the first
// run and to trakrf on every run afterwards (migration 000001 creates the
// schema). That produces two independent ledgers: the first records the real
// history, the second starts empty at version 0 and tries to replay the whole
// stack. The observable symptom is a table that silently never gets created
// while the ledger reports a clean, fully-applied version.
//
// "public" is the expected home: preview and prod were both verified to keep
// their ledger there, as does a fresh database's first run.
package migrate_test

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/buildinfo"
	"github.com/trakrf/platform/backend/internal/cmd/migrate"
)

// ledgerTestDB is a throwaway database, dropped and recreated per run. It is
// deliberately not trakrf_test — the shared harness database is managed by
// internal/testutil and this test needs to control provisioning itself.
const ledgerTestDB = "trakrf_ledger_test"

// obfuscationTestKey mirrors the fixed key used by internal/testutil: the
// Feistel id trigger reads it, and migration 000022 seeds rows.
const obfuscationTestKey = "6f626675736361746f72746573746b657920303132333435363738396162636465"

// adminURL returns a superuser URL for the maintenance database.
func adminURL(t *testing.T) string {
	t.Helper()

	pgURL := os.Getenv("PG_URL")
	if pgURL == "" {
		pgURL = "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	}
	// Tests run on the host; PG_URL may name the compose service.
	return strings.Replace(pgURL, "timescaledb", "localhost", 1)
}

// dbNameRE matches the database-name path segment of a postgres URL.
var dbNameRE = regexp.MustCompile(`^(postgres(?:ql)?://[^@]+@[^/]+/)[^?]+`)

// urlForDB rewrites the database name in a postgres URL, preserving the query
// string — including the options=-c search_path=trakrf,public that makes this
// bug reproducible.
func urlForDB(t *testing.T, base, dbName string) string {
	t.Helper()

	out := dbNameRE.ReplaceAllString(base, "${1}"+dbName)
	if out == base {
		t.Fatalf("could not rewrite database name in URL: %q", base)
	}
	return out
}

// provisionLedgerTestDB creates a pristine database that mirrors a fresh local
// bring-up: search_path points at trakrf first, but the trakrf schema does not
// exist yet.
func provisionLedgerTestDB(ctx context.Context, t *testing.T) string {
	t.Helper()

	admin := adminURL(t)

	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Skipf("no local postgres available (%v)", err)
	}
	defer conn.Close(ctx)

	for _, stmt := range []string{
		fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", ledgerTestDB),
		fmt.Sprintf("CREATE DATABASE %s", ledgerTestDB),
		fmt.Sprintf("ALTER DATABASE %s SET search_path TO trakrf, public", ledgerTestDB),
		fmt.Sprintf("ALTER DATABASE %s SET app.obfuscation_key = '%s'", ledgerTestDB, obfuscationTestKey),
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("provisioning %s failed (%q): %v", ledgerTestDB, stmt, err)
		}
	}

	t.Cleanup(func() {
		cleanupConn, err := pgx.Connect(context.Background(), admin)
		if err != nil {
			t.Logf("warning: could not connect to drop %s: %v", ledgerTestDB, err)
			return
		}
		defer cleanupConn.Close(context.Background())
		if _, err := cleanupConn.Exec(context.Background(),
			fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", ledgerTestDB)); err != nil {
			t.Logf("warning: could not drop %s: %v", ledgerTestDB, err)
		}
	})

	return urlForDB(t, admin, ledgerTestDB)
}

// ledgerSchemas returns every schema holding a schema_migrations table.
func ledgerSchemas(ctx context.Context, t *testing.T, dbURL string) []string {
	t.Helper()

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect to %s failed: %v", ledgerTestDB, err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, `
		SELECT table_schema
		FROM information_schema.tables
		WHERE table_name = 'schema_migrations'
		ORDER BY table_schema`)
	if err != nil {
		t.Fatalf("querying ledger location failed: %v", err)
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scanning ledger location failed: %v", err)
		}
		schemas = append(schemas, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating ledger locations failed: %v", err)
	}
	return schemas
}

// wantLedgerSchema is where the ledger must stay, on every run, forever.
const wantLedgerSchema = "public"

// TestLedgerStaysInOneSchema is the regression test for TRA-1069. Running
// migrate twice against a fresh database must leave exactly one ledger, in
// wantLedgerSchema, and the second run must be a clean no-op.
func TestLedgerStaysInOneSchema(t *testing.T) {
	ctx := context.Background()

	dbURL := provisionLedgerTestDB(ctx, t)
	t.Setenv("PG_URL", dbURL)

	if err := migrate.Run(ctx, buildinfo.Info{Version: "test"}); err != nil {
		t.Fatalf("first migrate run failed: %v", err)
	}

	if got := ledgerSchemas(ctx, t, dbURL); len(got) != 1 || got[0] != wantLedgerSchema {
		t.Fatalf("after first run: ledger schemas = %v, want [%s]", got, wantLedgerSchema)
	}

	// The second run is where the split shows up: once migration 000001 has
	// created the trakrf schema, CURRENT_SCHEMA() resolves differently than it
	// did on the first run.
	if err := migrate.Run(ctx, buildinfo.Info{Version: "test"}); err != nil {
		t.Fatalf("second migrate run failed (expected a no-op): %v", err)
	}

	if got := ledgerSchemas(ctx, t, dbURL); len(got) != 1 || got[0] != wantLedgerSchema {
		t.Fatalf("after second run: ledger schemas = %v, want [%s] "+
			"(a second ledger means the migration history was split)", got, wantLedgerSchema)
	}
}

// TestStrayLedgerIsReported covers the misleading-success half of TRA-1069: a
// database that already carries a ledger in the wrong schema must fail loudly
// rather than silently replaying or reporting a clean version.
func TestStrayLedgerIsReported(t *testing.T) {
	ctx := context.Background()

	dbURL := provisionLedgerTestDB(ctx, t)
	t.Setenv("PG_URL", dbURL)

	if err := migrate.Run(ctx, buildinfo.Info{Version: "test"}); err != nil {
		t.Fatalf("first migrate run failed: %v", err)
	}

	// Simulate the legacy state: a second ledger in trakrf, left behind by a run
	// whose CURRENT_SCHEMA() resolved to trakrf after migration 000001 created
	// the schema.
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		CREATE TABLE trakrf.schema_migrations
			(version bigint not null primary key, dirty boolean not null);
		INSERT INTO trakrf.schema_migrations (version, dirty) VALUES (10, false)`); err != nil {
		conn.Close(ctx)
		t.Fatalf("seeding stray ledger failed: %v", err)
	}
	conn.Close(ctx)

	err = migrate.Run(ctx, buildinfo.Info{Version: "test"})
	if err == nil {
		t.Fatal("migrate succeeded with a stray ledger in trakrf; want an error naming it")
	}
	if !strings.Contains(err.Error(), "trakrf.schema_migrations") {
		t.Fatalf("error should name the stray ledger and its schema, got: %v", err)
	}
	// The reported version is what makes the split diagnosable.
	if !strings.Contains(err.Error(), "version=10") {
		t.Fatalf("error should report the stray ledger's version, got: %v", err)
	}
}
