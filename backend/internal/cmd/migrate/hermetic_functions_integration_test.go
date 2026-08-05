//go:build integration
// +build integration

// TRA-1076 — a stored function must not depend on its caller's search_path.
//
// A function with no `SET search_path` of its own resolves unqualified names
// through the *caller's* session path. Every connection happens to carry
// search_path=trakrf,public today, so the whole stack works — but that makes a
// runtime connection parameter load-bearing rather than a convenience, and it is
// why TRA-1074 must replace the DSN setting rather than delete it.
//
// The concrete break is transitive and easy to miss. pgcrypto is installed into
// trakrf (migration 000001 creates it unqualified, and the runner's search_path
// puts it there), so trakrf._feistel_encrypt's unqualified hmac() call resolves
// only while trakrf is on the path. _feistel_encrypt backs
// trakrf.generate_obfuscated_id, which is the BEFORE INSERT id trigger on every
// table — so a hostile search_path breaks not just the three functions the ticket
// names but every INSERT in the schema.
//
// Beyond tidiness: an unqualified reference inside a SECURITY DEFINER function is
// a privilege-escalation vector, because a caller who can create objects in an
// earlier schema on the path can shadow the intended table and have it execute as
// the definer. Four of these functions are SECURITY DEFINER (ADR 0003).
package migrate_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/trakrf/platform/backend/internal/buildinfo"
	"github.com/trakrf/platform/backend/internal/cmd/migrate"
)

// hermeticByConstruction lists functions that are exempt from the pinned
// search_path requirement because their bodies reference nothing that a
// search_path could redirect — every name in them is schema-qualified, and the
// built-ins are written as pg_catalog.*.
//
// The exemption is not cosmetic. A `SET search_path` clause sets proconfig, and
// the planner refuses to inline any SQL function carrying one. normalize_tag_value
// is IMMUTABLE and deliberately inlinable: it backs the tags.normalized_value
// generated column and the per-read ingest membership query (storage/ingest.go),
// and migration 000017 chose SQL-over-plpgsql precisely to keep it inlinable.
// Qualifying its built-ins as pg_catalog.* buys full hermeticity at no planner
// cost, which is strictly better here than pinning the path.
//
// Add to this list only with the same justification: a body that cannot resolve
// anything through the path at all, plus a reason the pin would cost something.
var hermeticByConstruction = map[string]string{
	"normalize_tag_value": "pg_catalog-qualified built-ins only; must stay inlinable for the generated column and ingest hot path (000017)",
}

// unpinnedFunctions returns every function in trakrf that neither pins a
// search_path nor is owned by an extension.
//
// Extension-owned functions are excluded via pg_depend deptype 'e': pgcrypto
// lives in trakrf, so its ~40 functions would otherwise dominate the result, and
// their definitions are not ours to change.
func unpinnedFunctions(ctx context.Context, t *testing.T, conn *pgx.Conn) []string {
	t.Helper()

	rows, err := conn.Query(ctx, `
		SELECT p.proname || '(' || pg_get_function_identity_arguments(p.oid) || ')'
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'trakrf'
		  AND NOT EXISTS (
		      SELECT 1 FROM pg_depend d
		      WHERE d.objid = p.oid
		        AND d.classid = 'pg_proc'::regclass
		        AND d.deptype = 'e')
		  AND NOT EXISTS (
		      SELECT 1 FROM unnest(COALESCE(p.proconfig, '{}')) AS c
		      WHERE c LIKE 'search\_path=%')
		ORDER BY 1`)
	if err != nil {
		t.Fatalf("querying unpinned functions failed: %v", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var sig string
		if err := rows.Scan(&sig); err != nil {
			t.Fatalf("scanning function signature failed: %v", err)
		}
		out = append(out, sig)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating unpinned functions failed: %v", err)
	}
	return out
}

// funcName strips the argument list from a "name(args)" signature.
func funcName(sig string) string {
	for i, r := range sig {
		if r == '(' {
			return sig[:i]
		}
	}
	return sig
}

// migratedDB provisions a fresh database, runs the full migration stack against
// it, and returns its URL.
func migratedDB(ctx context.Context, t *testing.T) string {
	t.Helper()

	dbURL := provisionLedgerTestDB(ctx, t)
	t.Setenv("PG_URL", dbURL)

	if err := migrate.Run(ctx, buildinfo.Info{Version: "test"}); err != nil {
		t.Fatalf("migrate run failed: %v", err)
	}
	return dbURL
}

// TestFunctionsPinTheirSearchPath is the structural half: it fails for any
// function added later that forgets the pin, which is what stops this from
// regressing once the migration below has fixed the current set.
func TestFunctionsPinTheirSearchPath(t *testing.T) {
	ctx := context.Background()

	dbURL := migratedDB(ctx, t)

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer conn.Close(ctx)

	for _, sig := range unpinnedFunctions(ctx, t, conn) {
		if reason, ok := hermeticByConstruction[funcName(sig)]; ok {
			t.Logf("trakrf.%s is exempt: %s", sig, reason)
			continue
		}
		t.Errorf("trakrf.%s has no pinned search_path.\n"+
			"Add `SET search_path = trakrf, public` to its definition in a forward "+
			"migration and schema-qualify its body, so its behaviour cannot depend on "+
			"the caller's session (TRA-1076, ADR 0003). If it genuinely resolves "+
			"nothing through the path, add it to hermeticByConstruction with a reason.", sig)
	}
}

// TestFunctionsWorkUnderHostileSearchPath is the behavioural half, and is the
// verification the ticket specifies: exercise every schema-dependent path from a
// session whose search_path deliberately excludes trakrf. This fails before the
// migration and passes after.
func TestFunctionsWorkUnderHostileSearchPath(t *testing.T) {
	ctx := context.Background()

	dbURL := migratedDB(ctx, t)

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer conn.Close(ctx)

	// The hostile session. Everything below runs with trakrf off the path, so
	// any unqualified reference inside a function body fails to resolve.
	if _, err := conn.Exec(ctx, "SET search_path = public"); err != nil {
		t.Fatalf("setting hostile search_path failed: %v", err)
	}

	// The id trigger, reached directly. This is the root of the transitive
	// break: _feistel_encrypt calls hmac(), and pgcrypto lives in trakrf.
	var encrypted int64
	if err := conn.QueryRow(ctx, `SELECT trakrf._feistel_encrypt(1)`).Scan(&encrypted); err != nil {
		t.Fatalf("trakrf._feistel_encrypt failed under a hostile search_path: %v", err)
	}

	// And reached the way production reaches it: a plain INSERT firing the
	// BEFORE INSERT id trigger. Every table in the schema has one.
	var orgID int64
	if err := conn.QueryRow(ctx, `
		INSERT INTO trakrf.organizations (name, identifier)
		VALUES ('TRA-1076 hermetic', 'tra-1076-hermetic')
		RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("INSERT firing trakrf.generate_obfuscated_id failed under a hostile search_path: %v", err)
	}

	// The updated_at trigger.
	if _, err := conn.Exec(ctx, `
		UPDATE trakrf.organizations SET name = 'TRA-1076 hermetic (renamed)' WHERE id = $1`, orgID); err != nil {
		t.Fatalf("UPDATE firing trakrf.update_updated_at_column failed under a hostile search_path: %v", err)
	}

	// The two stored procedures the ticket names. Both insert into trakrf.tags,
	// so they also exercise the normalized_value generated column, which calls
	// normalize_tag_value.
	var assetID int64
	var assetTagIDs []int64
	if err := conn.QueryRow(ctx, `
		SELECT asset_id, tag_ids FROM trakrf.create_asset_with_tags(
			$1, 'tra-1076-asset', 'TRA-1076 asset', 'created under a hostile search_path',
			now(), NULL, true, '{}'::jsonb,
			'[{"type":"rfid","value":"000000000000000000010023"}]'::jsonb)`,
		orgID).Scan(&assetID, &assetTagIDs); err != nil {
		t.Fatalf("trakrf.create_asset_with_tags failed under a hostile search_path: %v", err)
	}
	if len(assetTagIDs) != 1 {
		t.Errorf("create_asset_with_tags returned %d tag ids, want 1", len(assetTagIDs))
	}

	var locationID int64
	var locationTagIDs []int64
	if err := conn.QueryRow(ctx, `
		SELECT location_id, tag_ids FROM trakrf.create_location_with_tags(
			$1, 'tra-1076-location', 'TRA-1076 location', NULL, NULL,
			now(), NULL, true, '{}'::jsonb,
			'[{"type":"rfid","value":"00ab12"}]'::jsonb)`,
		orgID).Scan(&locationID, &locationTagIDs); err != nil {
		t.Fatalf("trakrf.create_location_with_tags failed under a hostile search_path: %v", err)
	}
	if len(locationTagIDs) != 1 {
		t.Errorf("create_location_with_tags returned %d tag ids, want 1", len(locationTagIDs))
	}

	// The generated column really was computed, and by the shared normalizer:
	// the full-width EPC above must collapse to the short barcode form (TRA-944).
	var normalized string
	if err := conn.QueryRow(ctx, `
		SELECT normalized_value FROM trakrf.tags WHERE id = $1`, assetTagIDs[0]).Scan(&normalized); err != nil {
		t.Fatalf("reading tags.normalized_value failed under a hostile search_path: %v", err)
	}
	if normalized != "10023" {
		t.Errorf("tags.normalized_value = %q, want %q", normalized, "10023")
	}

	// normalize_tag_value called directly, and the functions that already pin
	// their path — a regression here would mean the migration broke them.
	var direct string
	if err := conn.QueryRow(ctx, `SELECT trakrf.normalize_tag_value('000000000000000000010023')`).Scan(&direct); err != nil {
		t.Fatalf("trakrf.normalize_tag_value failed under a hostile search_path: %v", err)
	}
	if direct != "10023" {
		t.Errorf("trakrf.normalize_tag_value = %q, want %q", direct, "10023")
	}

	var entitled bool
	if err := conn.QueryRow(ctx, `SELECT trakrf.org_is_entitled($1)`, orgID).Scan(&entitled); err != nil {
		t.Fatalf("trakrf.org_is_entitled failed under a hostile search_path: %v", err)
	}

	var capabilities []string
	if err := conn.QueryRow(ctx, `SELECT trakrf.org_capability_set($1)`, orgID).Scan(&capabilities); err != nil {
		t.Fatalf("trakrf.org_capability_set failed under a hostile search_path: %v", err)
	}

	for _, q := range []string{
		`SELECT count(*) FROM trakrf.list_active_scan_topics()`,
		`SELECT count(*) FROM trakrf.resolve_scan_topic('tra-1076/no/such/topic')`,
	} {
		var n int64
		if err := conn.QueryRow(ctx, q).Scan(&n); err != nil {
			t.Fatalf("%s failed under a hostile search_path: %v", q, err)
		}
	}
}

// TestFunctionsSurviveAnEmptySearchPath is the stronger form of the same claim.
// `SET search_path = public` still leaves pg_catalog implicitly first; an empty
// path removes even that fallback, so anything that resolves here is resolving
// through the function's own pin rather than through the session at all.
func TestFunctionsSurviveAnEmptySearchPath(t *testing.T) {
	ctx := context.Background()

	dbURL := migratedDB(ctx, t)

	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `SET search_path = ''`); err != nil {
		t.Fatalf("clearing search_path failed: %v", err)
	}

	var orgID int64
	if err := conn.QueryRow(ctx, `
		INSERT INTO trakrf.organizations (name, identifier)
		VALUES ('TRA-1076 empty path', 'tra-1076-empty-path')
		RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("INSERT firing the id trigger failed with an empty search_path: %v", err)
	}

	var assetID int64
	var tagIDs []int64
	if err := conn.QueryRow(ctx, fmt.Sprintf(`
		SELECT asset_id, tag_ids FROM trakrf.create_asset_with_tags(
			%d, 'tra-1076-empty', 'TRA-1076 empty path', NULL,
			pg_catalog.now(), NULL, true, '{}'::pg_catalog.jsonb,
			'[{"type":"rfid","value":"00ab12"}]'::pg_catalog.jsonb)`, orgID)).Scan(&assetID, &tagIDs); err != nil {
		t.Fatalf("trakrf.create_asset_with_tags failed with an empty search_path: %v", err)
	}
	if len(tagIDs) != 1 {
		t.Errorf("create_asset_with_tags returned %d tag ids, want 1", len(tagIDs))
	}
}
