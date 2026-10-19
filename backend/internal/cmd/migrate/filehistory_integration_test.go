//go:build integration
// +build integration

// The applied-file record: a version the database has run must still be the
// file the image carries at that version, or migrate refuses before writing.
//
// These tests hand runURL small fixture images instead of the embedded set, so
// they can replay what happened on preview — a version applied by one build and
// reused or removed by the next — without TimescaleDB or the real stack.
package migrate

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/trakrf/platform/backend/internal/buildinfo"
)

const fileHistoryTestDB = "trakrf_file_history_test"

// provisionFileHistoryDB creates an empty database and returns its URL plus a
// superuser pool on it for assertions.
func provisionFileHistoryDB(ctx context.Context, t *testing.T) (string, *pgxpool.Pool) {
	t.Helper()

	admin := ownershipAdminURL(t)
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		if allowDBSkip() {
			t.Skipf("no local postgres available (%v); skipping because %s=1", err, allowDBSkipEnv)
		}
		t.Fatalf("no local postgres available (%v).\n"+
			"Start one with `just database up`, or set %s=1 to skip instead.",
			err, allowDBSkipEnv)
	}
	defer conn.Close(ctx)

	for _, stmt := range []string{
		fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", fileHistoryTestDB),
		fmt.Sprintf("CREATE DATABASE %s", fileHistoryTestDB),
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("provisioning failed (%q): %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), admin)
		if err != nil {
			t.Logf("warning: cleanup connect failed: %v", err)
			return
		}
		defer c.Close(context.Background())
		if _, err := c.Exec(context.Background(),
			fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", fileHistoryTestDB)); err != nil {
			t.Logf("warning: cleanup failed: %v", err)
		}
	})

	u, err := url.Parse(admin)
	if err != nil {
		t.Fatalf("parsing admin URL failed: %v", err)
	}
	u.Path = "/" + fileHistoryTestDB
	return u.String(), poolAs(ctx, t, admin, fileHistoryTestDB, "", "")
}

// table is a migration that creates one table, idempotently.
func table(name string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS trakrf." + name + " (id int);")}
}

func migrateWith(ctx context.Context, dbURL string, fsys fstest.MapFS) error {
	return runURL(ctx, dbURL, buildinfo.Info{Version: "test"}, fsys)
}

type ledgerState struct {
	version int64
	dirty   bool
	files   map[int64]string
}

func readState(ctx context.Context, t *testing.T, pool *pgxpool.Pool) ledgerState {
	t.Helper()
	var s ledgerState
	if err := pool.QueryRow(ctx,
		"SELECT version, dirty FROM trakrf.schema_migrations").Scan(&s.version, &s.dirty); err != nil {
		t.Fatalf("reading ledger: %v", err)
	}
	s.files = map[int64]string{}
	rows, err := pool.Query(ctx, "SELECT version, filename FROM trakrf.schema_migration_files")
	if err != nil {
		t.Fatalf("reading file history: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int64
		var f string
		if err := rows.Scan(&v, &f); err != nil {
			t.Fatalf("reading file history: %v", err)
		}
		s.files[v] = f
	}
	return s
}

func tableExists(ctx context.Context, t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_tables WHERE schemaname = 'trakrf' AND tablename = $1)",
		name).Scan(&ok); err != nil {
		t.Fatalf("checking table %s: %v", name, err)
	}
	return ok
}

func TestFileHistory_RecordsEachAppliedVersion(t *testing.T) {
	ctx := context.Background()
	dbURL, pool := provisionFileHistoryDB(ctx, t)

	image := fstest.MapFS{
		"000001_base.up.sql":  table("base"),
		"000002_items.up.sql": table("items"),
	}
	if err := migrateWith(ctx, dbURL, image); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	image["000003_tags.up.sql"] = table("tags")
	if err := migrateWith(ctx, dbURL, image); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	s := readState(ctx, t, pool)
	if s.version != 3 || len(s.files) != 3 || s.files[3] != "000003_tags.up.sql" {
		t.Fatalf("expected versions 1-3 recorded, got %+v", s)
	}

	// The same image again is a no-op, not a refusal.
	if err := migrateWith(ctx, dbURL, image); err != nil {
		t.Fatalf("re-running the same image must succeed: %v", err)
	}
}

// A database migrated before the history existed — prod, preview — is seeded
// from the image on its first run under this code, even with nothing pending.
func TestFileHistory_SeedsTrustOnFirstUse(t *testing.T) {
	ctx := context.Background()
	dbURL, pool := provisionFileHistoryDB(ctx, t)

	image := fstest.MapFS{
		"000001_base.up.sql":  table("base"),
		"000002_items.up.sql": table("items"),
	}
	if err := migrateWith(ctx, dbURL, image); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, "DROP TABLE trakrf.schema_migration_files"); err != nil {
		t.Fatalf("simulating a pre-history database: %v", err)
	}

	if err := migrateWith(ctx, dbURL, image); err != nil {
		t.Fatalf("migrate with nothing pending: %v", err)
	}
	s := readState(ctx, t, pool)
	if len(s.files) != 2 || s.files[1] != "000001_base.up.sql" || s.files[2] != "000002_items.up.sql" {
		t.Fatalf("expected 1-2 seeded from the image, got %+v", s.files)
	}
}

// Preview on 10-02: a PR applied 2 and left; main then landed a different 2 and
// a 3. golang-migrate continued from 3 and never ran main's 2. Now it refuses,
// and nothing is written — not 3, not the ledger, not the history.
func TestFileHistory_RefusesAReusedNumberBeforeWriting(t *testing.T) {
	ctx := context.Background()
	dbURL, pool := provisionFileHistoryDB(ctx, t)

	if err := migrateWith(ctx, dbURL, fstest.MapFS{
		"000001_base.up.sql":           table("base"),
		"000002_pr_offboarding.up.sql": table("pr_offboarding"),
	}); err != nil {
		t.Fatalf("migrate the PR's image: %v", err)
	}
	before := readState(ctx, t, pool)

	err := migrateWith(ctx, dbURL, fstest.MapFS{
		"000001_base.up.sql":            table("base"),
		"000002_email_callbacks.up.sql": table("email_callbacks"),
		"000003_river.up.sql":           table("river"),
	})
	if err == nil {
		t.Fatal("expected a reused version to be refused")
	}
	t.Logf("operator sees:\n%v", err)
	for _, want := range []string{"000002_pr_offboarding.up.sql", "000002_email_callbacks.up.sql", "UPDATE trakrf.schema_migration_files"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal missing %q", want)
		}
	}

	after := readState(ctx, t, pool)
	if after.version != before.version || after.dirty || len(after.files) != len(before.files) {
		t.Errorf("refusal wrote something: before %+v, after %+v", before, after)
	}
	if tableExists(ctx, t, pool, "river") || tableExists(ctx, t, pool, "email_callbacks") {
		t.Error("refusal applied a migration")
	}
}

func TestFileHistory_RefusesAnEditedFile(t *testing.T) {
	ctx := context.Background()
	dbURL, pool := provisionFileHistoryDB(ctx, t)

	if err := migrateWith(ctx, dbURL, fstest.MapFS{"000001_base.up.sql": table("base")}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	err := migrateWith(ctx, dbURL, fstest.MapFS{"000001_base.up.sql": table("base_renamed")})
	if err == nil || !strings.Contains(err.Error(), "version 1") {
		t.Fatalf("expected the edited file refused, got %v", err)
	}
	_ = pool
}

// Preview from 09-28: the build that applied 2 no longer carries it.
func TestFileHistory_RefusesAMissingFile(t *testing.T) {
	ctx := context.Background()
	dbURL, pool := provisionFileHistoryDB(ctx, t)

	if err := migrateWith(ctx, dbURL, fstest.MapFS{
		"000001_base.up.sql":       table("base"),
		"000002_pr_notices.up.sql": table("pr_notices"),
	}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	before := readState(ctx, t, pool)

	err := migrateWith(ctx, dbURL, fstest.MapFS{"000001_base.up.sql": table("base")})
	if err == nil {
		t.Fatal("expected a missing applied file to be refused")
	}
	t.Logf("operator sees:\n%v", err)
	if !strings.Contains(err.Error(), "000002_pr_notices.up.sql") ||
		!strings.Contains(err.Error(), "this image has no file 2") {
		t.Errorf("refusal should name the applied file and its absence, got:\n%v", err)
	}
	if after := readState(ctx, t, pool); after.version != before.version || len(after.files) != len(before.files) {
		t.Errorf("refusal wrote something: before %+v, after %+v", before, after)
	}
}

// The UPDATE in the refusal must be executable as printed and must clear it,
// or the guard is a dead end rather than a gate.
func TestFileHistory_PrintedRepairClearsTheRefusal(t *testing.T) {
	ctx := context.Background()
	dbURL, pool := provisionFileHistoryDB(ctx, t)

	if err := migrateWith(ctx, dbURL, fstest.MapFS{"000001_base.up.sql": table("base")}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// A renumber that kept the bytes, as preview's 000049 did: the schema already
	// reflects the image's file, so re-pointing the record is the whole repair.
	renamed := fstest.MapFS{"000001_base_renumbered.up.sql": table("base")}
	err := migrateWith(ctx, dbURL, renamed)
	if err == nil {
		t.Fatal("expected the renamed file refused")
	}

	var repair string
	for _, line := range strings.Split(err.Error(), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "UPDATE ") {
			repair = strings.TrimSpace(line)
		}
	}
	if repair == "" {
		t.Fatalf("no repair statement in:\n%v", err)
	}
	if _, err := pool.Exec(ctx, repair); err != nil {
		t.Fatalf("printed repair is not executable: %v\n%s", err, repair)
	}
	if err := migrateWith(ctx, dbURL, renamed); err != nil {
		t.Fatalf("repair did not clear the refusal: %v", err)
	}
}

// The application must not be able to rewrite the record this check trusts,
// even though the schema's default privileges grant it write on new tables.
func TestFileHistory_AppRoleCannotWriteTheHistory(t *testing.T) {
	ctx := context.Background()
	dbURL, pool := provisionFileHistoryDB(ctx, t)

	const appRole = "tra_file_history_app"
	for _, stmt := range []string{
		"DROP ROLE IF EXISTS " + appRole,
		"CREATE ROLE " + appRole + " NOSUPERUSER",
		"CREATE SCHEMA trakrf",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA trakrf GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO " + appRole,
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("setup failed (%q): %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP OWNED BY "+appRole)
		_, _ = pool.Exec(context.Background(), "DROP ROLE IF EXISTS "+appRole)
	})

	if err := migrateWith(ctx, dbURL, fstest.MapFS{"000001_base.up.sql": table("base")}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for priv, want := range map[string]bool{"SELECT": true, "INSERT": false, "UPDATE": false, "DELETE": false} {
		var has bool
		if err := pool.QueryRow(ctx,
			"SELECT has_table_privilege($1, 'trakrf.schema_migration_files', $2)", appRole, priv).Scan(&has); err != nil {
			t.Fatalf("checking %s: %v", priv, err)
		}
		if has != want {
			t.Errorf("app role %s on the file history = %t, want %t", priv, has, want)
		}
	}
	// The ordinary table keeps the default grant; the revoke is targeted.
	var appWrites bool
	if err := pool.QueryRow(ctx,
		"SELECT has_table_privilege($1, 'trakrf.base', 'INSERT')", appRole).Scan(&appWrites); err != nil {
		t.Fatalf("checking base: %v", err)
	}
	if !appWrites {
		t.Error("the revoke reached an application table")
	}
}
