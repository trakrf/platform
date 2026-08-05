//go:build integration
// +build integration

// TRA-1104 — the migrating role must own everything it might have to replace.
//
// These tests live in package migrate rather than migrate_test because the audit
// query is the unit under test and is deliberately unexported; asserting on it
// through Run would only tell us that *something* refused, not that the catalogue
// sweep sees the right objects.
package migrate

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/trakrf/platform/backend/internal/buildinfo"
)

// These mirror internal/testutil rather than importing it: testutil imports this
// package, so an in-package test that pulled it in would be an import cycle.
// Kept deliberately literal, including the env var names, so a change there is a
// visible mismatch here rather than a silent divergence.
const allowDBSkipEnv = "TRAKRF_ALLOW_DB_SKIP"

// allowDBSkip reports whether a test may skip itself when no database is
// reachable. It may not, by default — TRA-1085, where this suite exited 0 while
// every test in it skipped on a bad password.
func allowDBSkip() bool { return os.Getenv(allowDBSkipEnv) == "1" }

const (
	ownershipTestDB    = "trakrf_ownership_test"
	migrateRole        = "tra1104_migrate"
	otherRole          = "tra1104_other"
	rolePassword       = "tra1104"
	driftedFunctionSQL = `CREATE FUNCTION trakrf.drifted_probe(v text) RETURNS text
	                      LANGUAGE sql IMMUTABLE AS $$ SELECT v $$`
)

// ownershipAdminURL returns a superuser URL for the maintenance database. It
// reads PG_ADMIN_URL, not PG_URL: since TRA-1075 PG_URL is the non-superuser
// trakrf-app role, which cannot create the roles these tests need. The hostname
// rewrite matters because the tests run on the host, not in compose.
func ownershipAdminURL(t *testing.T) string {
	t.Helper()

	if u := os.Getenv("PG_ADMIN_URL"); u != "" {
		return strings.Replace(u, "timescaledb", "localhost", 1)
	}
	return "postgresql://postgres:postgres@localhost:5432/postgres?sslmode=disable"
}

// provisionOwnershipDB builds a throwaway database holding a trakrf schema owned
// by migrateRole, plus one function left owned by otherRole — the drift this
// preflight exists to catch. It returns a pool connected *as migrateRole*, which
// is what a real migrate Job looks like.
func provisionOwnershipDB(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()

	admin := ownershipAdminURL(t)

	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		// TRA-1085: fail rather than skip. A suite that passes by not running is
		// worse than no suite.
		if allowDBSkip() {
			t.Skipf("no local postgres available (%v); skipping because %s=1", err, allowDBSkipEnv)
		}
		t.Fatalf("no local postgres available (%v).\n"+
			"Start one with `just database up`, or set %s=1 to skip instead.",
			err, allowDBSkipEnv)
	}
	defer conn.Close(ctx)

	for _, stmt := range []string{
		fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", ownershipTestDB),
		fmt.Sprintf("DROP ROLE IF EXISTS %s", migrateRole),
		fmt.Sprintf("DROP ROLE IF EXISTS %s", otherRole),
		fmt.Sprintf("CREATE ROLE %s NOSUPERUSER LOGIN PASSWORD '%s'", migrateRole, rolePassword),
		fmt.Sprintf("CREATE ROLE %s NOSUPERUSER", otherRole),
		fmt.Sprintf("CREATE DATABASE %s", ownershipTestDB),
		// The real trakrf-migrate can create the schema; without this the runner
		// would fail on CREATE SCHEMA for an unrelated reason and the test would
		// pass for the wrong one.
		fmt.Sprintf("GRANT CREATE ON DATABASE %s TO %s", ownershipTestDB, migrateRole),
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
		for _, stmt := range []string{
			fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", ownershipTestDB),
			fmt.Sprintf("DROP ROLE IF EXISTS %s", migrateRole),
			fmt.Sprintf("DROP ROLE IF EXISTS %s", otherRole),
		} {
			if _, err := c.Exec(context.Background(), stmt); err != nil {
				t.Logf("warning: cleanup %q failed: %v", stmt, err)
			}
		}
	})

	// Build the schema as the superuser, exactly as a hand-run session would.
	dbAsAdmin := poolAs(ctx, t, admin, ownershipTestDB, "", "")
	defer dbAsAdmin.Close()

	for _, stmt := range []string{
		"CREATE SCHEMA trakrf",
		"CREATE TABLE trakrf.owned_table (id int)",
		"CREATE VIEW trakrf.owned_view AS SELECT 1 AS one",
		driftedFunctionSQL,
		// Everything belongs to the migrating role...
		fmt.Sprintf("ALTER SCHEMA trakrf OWNER TO %s", migrateRole),
		fmt.Sprintf("ALTER TABLE trakrf.owned_table OWNER TO %s", migrateRole),
		fmt.Sprintf("ALTER VIEW trakrf.owned_view OWNER TO %s", migrateRole),
		// ...except this one, which is the whole point.
		fmt.Sprintf("ALTER FUNCTION trakrf.drifted_probe(text) OWNER TO %s", otherRole),
	} {
		if _, err := dbAsAdmin.Exec(ctx, stmt); err != nil {
			t.Fatalf("seeding failed (%q): %v", stmt, err)
		}
	}

	return poolAs(ctx, t, admin, ownershipTestDB, migrateRole, rolePassword)
}

// poolAs opens a pool against dbName, optionally overriding the role. An empty
// user keeps whatever the base URL carries.
//
// It imposes the runner's own search_path, and that is load-bearing rather than
// incidental: regprocedure renders a function name unqualified exactly when its
// schema is on the caller's path, so a harness that skipped this would report
// qualified names the real runner never produces — and a test asserting
// qualification would pass while the shipped message was wrong.
func poolAs(ctx context.Context, t *testing.T, baseURL, dbName, user, password string) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(baseURL)
	if err != nil {
		t.Fatalf("parsing base URL failed: %v", err)
	}
	cfg.ConnConfig.Database = dbName
	cfg.ConnConfig.RuntimeParams["search_path"] = ddlSearchPath
	if user != "" {
		cfg.ConnConfig.User = user
		cfg.ConnConfig.Password = password
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connecting as %q failed: %v", user, err)
	}
	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping as %q failed: %v", user, err)
	}
	return pool
}

// TestFindOwnershipDrift_CatchesForeignOwnedObject is the TRA-1104 regression:
// an object created out of band by a superuser is invisible to every migration
// until one tries to replace it, at which point the deploy wedges.
func TestFindOwnershipDrift_CatchesForeignOwnedObject(t *testing.T) {
	ctx := context.Background()
	pool := provisionOwnershipDB(ctx, t)

	drifts, err := findOwnershipDrift(ctx, pool)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}

	if len(drifts) != 1 {
		t.Fatalf("expected exactly 1 drifted object, got %d: %+v", len(drifts), drifts)
	}
	got := drifts[0]
	if !strings.Contains(got.name, "drifted_probe") {
		t.Errorf("name = %q, want it to name drifted_probe", got.name)
	}
	if got.kind != "function" {
		t.Errorf("kind = %q, want %q", got.kind, "function")
	}
	if got.owner != otherRole {
		t.Errorf("owner = %q, want %q", got.owner, otherRole)
	}
}

// Names must be schema-qualified, always. The repair line gets pasted into a
// psql session whose search_path is not the runner's — `just ops psql` opens on
// the default path — and an unqualified ALTER FUNCTION there either misses or,
// worse, hits a same-named function in another schema.
//
// regprocedure is the trap: it renders unqualified whenever the schema is in the
// caller's search_path, and the runner sets search_path=trakrf,public on every
// connection it opens. So the one context where this audit runs is exactly the
// context where regprocedure drops the qualification.
func TestFindOwnershipDrift_NamesAreSchemaQualified(t *testing.T) {
	ctx := context.Background()
	pool := provisionOwnershipDB(ctx, t)

	drifts, err := findOwnershipDrift(ctx, pool)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if len(drifts) == 0 {
		t.Fatal("expected drift to assert on")
	}
	for _, d := range drifts {
		if !strings.HasPrefix(d.name, appSchema+".") {
			t.Errorf("name %q is not schema-qualified; the repair statement would be "+
				"wrong in any session whose search_path differs from the runner's", d.name)
		}
	}
}

// The objects the migrating role does own must not be reported. A preflight that
// cries wolf on a correctly-owned schema gets disabled, and then guards nothing.
func TestFindOwnershipDrift_IgnoresCorrectlyOwnedObjects(t *testing.T) {
	ctx := context.Background()
	pool := provisionOwnershipDB(ctx, t)

	drifts, err := findOwnershipDrift(ctx, pool)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}

	for _, d := range drifts {
		if strings.Contains(d.name, "owned_table") ||
			strings.Contains(d.name, "owned_view") ||
			d.kind == "schema" {
			t.Errorf("reported a correctly-owned object as drift: %+v", d)
		}
	}
}

// Repairing ownership must clear the refusal, or the guard is a dead end rather
// than a gate — this is the "then re-run migrate" half of the message.
func TestFindOwnershipDrift_ClearsAfterRepair(t *testing.T) {
	ctx := context.Background()
	pool := provisionOwnershipDB(ctx, t)

	admin := poolAs(ctx, t, ownershipAdminURL(t), ownershipTestDB, "", "")
	if _, err := admin.Exec(ctx,
		fmt.Sprintf("ALTER FUNCTION trakrf.drifted_probe(text) OWNER TO %s", migrateRole)); err != nil {
		t.Fatalf("repair failed: %v", err)
	}

	drifts, err := findOwnershipDrift(ctx, pool)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if len(drifts) != 0 {
		t.Fatalf("expected no drift after repair, got %+v", drifts)
	}
}

// The repair statement in the message must be executable as printed. Asserting
// on its text only proves it is well-formed-looking; running it proves it is
// valid DDL that actually resolves the object and clears the refusal.
//
// This is the assertion that would have caught the unqualified-name defect, and
// it also pins the argument-list rendering: identity arguments come out as
// "(v text)" rather than "(text)", which is legal in an ALTER FUNCTION signature
// but is not obviously so by inspection.
func TestOwnershipRepairStatementActuallyWorks(t *testing.T) {
	ctx := context.Background()
	pool := provisionOwnershipDB(ctx, t)

	drifts, err := findOwnershipDrift(ctx, pool)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drifted object, got %+v", drifts)
	}

	// Rebuild exactly the statement the operator is told to run.
	d := drifts[0]
	stmt := fmt.Sprintf("%s %s OWNER TO %s;", alterVerb(d.kind), d.name,
		pgx.Identifier{migrateRole}.Sanitize())
	t.Logf("executing the printed repair: %s", stmt)

	admin := poolAs(ctx, t, ownershipAdminURL(t), ownershipTestDB, "", "")
	if _, err := admin.Exec(ctx, stmt); err != nil {
		t.Fatalf("the repair statement in the refusal is not executable: %v\nstatement: %s", err, stmt)
	}

	after, err := findOwnershipDrift(ctx, pool)
	if err != nil {
		t.Fatalf("re-audit failed: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("repair ran but drift remains: %+v", after)
	}
}

// Postgres accepts an ownership check when the current role is a member of the
// owning role. Reporting that as drift would fail a deploy that would in fact
// have succeeded — the worst possible false positive for a preflight.
func TestFindOwnershipDrift_RoleMembershipCountsAsOwnership(t *testing.T) {
	ctx := context.Background()
	pool := provisionOwnershipDB(ctx, t)

	admin := poolAs(ctx, t, ownershipAdminURL(t), ownershipTestDB, "", "")
	if _, err := admin.Exec(ctx,
		fmt.Sprintf("GRANT %s TO %s", otherRole, migrateRole)); err != nil {
		t.Fatalf("granting role membership failed: %v", err)
	}

	drifts, err := findOwnershipDrift(ctx, pool)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if len(drifts) != 0 {
		t.Fatalf("membership in the owning role should count as ownership, got %+v", drifts)
	}
}

// urlAs rebuilds the admin URL pointed at dbName as the given role, preserving
// host, port and query string.
func urlAs(t *testing.T, dbName, user, password string) string {
	t.Helper()

	u, err := url.Parse(ownershipAdminURL(t))
	if err != nil {
		t.Fatalf("parsing admin URL failed: %v", err)
	}
	u.User = url.UserPassword(user, password)
	u.Path = "/" + dbName
	return u.String()
}

// TestRunURL_RefusesOnOwnershipDrift is the end-to-end guarantee: the wedge is
// prevented, not merely detectable. RunURL must refuse *before* golang-migrate
// gets a chance to record anything, so the ledger stays clean and the running pod
// keeps serving — the failure mode TRA-1104 did not have.
func TestRunURL_RefusesOnOwnershipDrift(t *testing.T) {
	ctx := context.Background()
	provisionOwnershipDB(ctx, t) // seeds trakrf with one foreign-owned function

	err := RunURL(ctx, urlAs(t, ownershipTestDB, migrateRole, rolePassword),
		buildinfo.Info{Version: "test"})
	if err == nil {
		t.Fatal("expected RunURL to refuse on ownership drift, got nil")
	}
	// The refusal is the deliverable, so put it in front of anyone running -v.
	t.Logf("operator sees:\n%v", err)

	for _, want := range []string{"refusing to migrate", "drifted_probe", "ALTER FUNCTION"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal missing %q, got:\n%v", want, err)
		}
	}

	// Failing safe means leaving no trace: no ledger, so nothing to reconcile by
	// hand and nothing that makes the next run behave differently.
	admin := poolAs(ctx, t, ownershipAdminURL(t), ownershipTestDB, "", "")
	var ledgers int
	if err := admin.QueryRow(ctx,
		"SELECT count(*) FROM pg_tables WHERE tablename = 'schema_migrations'").Scan(&ledgers); err != nil {
		t.Fatalf("counting ledgers failed: %v", err)
	}
	if ledgers != 0 {
		t.Errorf("preflight ran too late: %d ledger table(s) created before refusing", ledgers)
	}
}

// A superuser can replace anything regardless of owner, so there is no drift to
// report. Local development and the integration harness both migrate as
// postgres; reporting drift there would break every developer's `just db reset`
// for a condition that cannot affect them.
func TestFindOwnershipDrift_SuperuserIsExempt(t *testing.T) {
	ctx := context.Background()
	provisionOwnershipDB(ctx, t) // seeds the drifted function

	asSuper := poolAs(ctx, t, ownershipAdminURL(t), ownershipTestDB, "", "")

	drifts, err := findOwnershipDrift(ctx, asSuper)
	if err != nil {
		t.Fatalf("audit failed: %v", err)
	}
	if len(drifts) != 0 {
		t.Fatalf("superuser should see no drift, got %+v", drifts)
	}
}
