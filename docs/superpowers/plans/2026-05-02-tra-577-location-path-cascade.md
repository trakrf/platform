# TRA-577 — `location.path` backfill + cascade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a single migration that (a) introduces an AFTER cascade trigger so re-parenting a location updates descendant `path` values, and (b) backfills all legacy non-canonical `path` rows via a reusable PL/pgSQL function that future migrations can call.

**Architecture:** Two PL/pgSQL artifacts in one migration: `trakrf.cascade_location_path()` (paired AFTER UPDATE trigger function) and `trakrf.recompute_location_paths()` (idempotent walker called once at migration time). All discovery/verification before code via TDD. Tests are integration-tagged because the trigger behavior is database-resident.

**Tech Stack:** PostgreSQL 15+ with `ltree`, Go 1.22 (integration tests), `golang-migrate` (versioned SQL migrations under `backend/migrations/`).

**Spec:** `docs/superpowers/specs/2026-05-02-tra-577-location-path-cascade-design.md`

---

## File Structure

**New files:**
- `backend/migrations/000038_location_path_backfill_and_cascade.up.sql` — migration: cascade function + trigger, recompute function, one-time recompute call.
- `backend/migrations/000038_location_path_backfill_and_cascade.down.sql` — drop the cascade trigger, both functions. (Down does NOT undo the data backfill — irreversible by design; we don't have the old non-canonical paths anywhere.)
- `backend/migrations/CONVENTIONS.md` — short doc with the path-recompute rule.
- `backend/internal/storage/locations_path_cascade_integration_test.go` — four integration tests: re-parent cascade, external_key cascade, recompute idempotency, recompute fixes legacy rows.

**Modified files:** none in Go source. The trigger does the work; existing `UpdateLocation` already issues `UPDATE OF parent_location_id, external_key`, which is what fires both triggers.

---

## Task 1: Write the four integration tests (failing)

**Files:**
- Create: `backend/internal/storage/locations_path_cascade_integration_test.go`

- [ ] **Step 1.1: Create the integration test file with all four tests**

Write to `backend/internal/storage/locations_path_cascade_integration_test.go`:

```go
//go:build integration
// +build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trakrf/platform/backend/internal/models/location"
	"github.com/trakrf/platform/backend/internal/testutil"
)

// mkLoc inserts a location through the storage API so triggers fire as in
// production. Returns the assigned id and the resulting canonical path.
func mkLoc(t *testing.T, ctx context.Context, store interface {
	CreateLocation(context.Context, location.Location) (*location.Location, error)
}, orgID int, externalKey string, parentID *int) *location.Location {
	t.Helper()
	loc, err := store.CreateLocation(ctx, location.Location{
		OrgID:       orgID,
		ExternalKey: externalKey,
		Name:        externalKey,
		ParentID:    parentID,
		ValidFrom:   time.Now(),
		IsActive:    true,
	})
	require.NoError(t, err)
	require.NotNil(t, loc)
	return loc
}

// TestUpdateLocation_Reparent_CascadesDescendants exercises the BB15 cascade
// gotcha: moving B (parent of C) under D must update C.path too.
func TestUpdateLocation_Reparent_CascadesDescendants(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	a := mkLoc(t, ctx, store, orgID, "a", nil)
	b := mkLoc(t, ctx, store, orgID, "b", &a.ID)
	c := mkLoc(t, ctx, store, orgID, "c", &b.ID)
	d := mkLoc(t, ctx, store, orgID, "d", nil)

	require.Equal(t, "a", a.Path)
	require.Equal(t, "a.b", b.Path)
	require.Equal(t, "a.b.c", c.Path)
	require.Equal(t, "d", d.Path)

	updated, err := store.UpdateLocation(ctx, orgID, b.ID, location.UpdateLocationRequest{
		ParentID: &d.ID,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "d.b", updated.Path, "B's own path must reflect new parent")

	cAfter, err := store.GetLocationByID(ctx, orgID, c.ID)
	require.NoError(t, err)
	require.NotNil(t, cAfter)
	assert.Equal(t, "d.b.c", cAfter.Path, "C must follow B under its new parent")

	descendants, err := store.GetDescendants(ctx, orgID, d.ID)
	require.NoError(t, err)
	ids := make(map[int]bool)
	for _, loc := range descendants {
		ids[loc.ID] = true
	}
	assert.True(t, ids[b.ID], "GetDescendants(D) must include B")
	assert.True(t, ids[c.ID], "GetDescendants(D) must include C after cascade")
}

// TestUpdateLocation_ChangeExternalKey_CascadesDescendants verifies the
// cascade path also fires on external_key rename, not just re-parent.
func TestUpdateLocation_ChangeExternalKey_CascadesDescendants(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	a := mkLoc(t, ctx, store, orgID, "a", nil)
	b := mkLoc(t, ctx, store, orgID, "b", &a.ID)
	c := mkLoc(t, ctx, store, orgID, "c", &b.ID)

	newKey := "b2"
	updated, err := store.UpdateLocation(ctx, orgID, b.ID, location.UpdateLocationRequest{
		ExternalKey: &newKey,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, "a.b2", updated.Path)

	cAfter, err := store.GetLocationByID(ctx, orgID, c.ID)
	require.NoError(t, err)
	require.NotNil(t, cAfter)
	assert.Equal(t, "a.b2.c", cAfter.Path)
}

// TestRecomputeLocationPaths_Idempotent confirms the recompute function is a
// no-op on an already-canonical tree. Future migrations that touch path
// semantics can call it safely without corrupting good data.
func TestRecomputeLocationPaths_Idempotent(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	a := mkLoc(t, ctx, store, orgID, "wh-1", nil)
	mkLoc(t, ctx, store, orgID, "bay-3", &a.ID)

	var rows int
	err := pool.QueryRow(ctx, "SELECT trakrf.recompute_location_paths()").Scan(&rows)
	require.NoError(t, err)
	assert.Equal(t, 0, rows, "recompute on canonical tree must update zero rows")
}

// TestRecomputeLocationPaths_FixesLegacyRows simulates the BB15 D-2 finding:
// a row that bypassed the trigger and holds a non-canonical path. After
// recompute, the row matches the canonical rule and ancestor/descendant
// queries return correct results.
func TestRecomputeLocationPaths_FixesLegacyRows(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)

	parent := mkLoc(t, ctx, store, orgID, "WHS-01", nil)
	require.Equal(t, "whs_01", parent.Path)

	// Insert a child while bypassing the BEFORE trigger so we can plant
	// a deliberately non-canonical legacy path. Mirrors the preview-DB
	// state where 704/713 rows have stale paths.
	_, err := pool.Exec(ctx, "ALTER TABLE trakrf.locations DISABLE TRIGGER maintain_location_path")
	require.NoError(t, err)

	var childID int
	err = pool.QueryRow(ctx, `
		INSERT INTO trakrf.locations
		(name, external_key, parent_location_id, path, valid_from, is_active, org_id)
		VALUES ($1, $2, $3, $4::ltree, $5, $6, $7)
		RETURNING id
	`, "WHS-07-03", "WHS-07-03", parent.ID, "WHS-01.WHS-07-03", time.Now(), true, orgID).Scan(&childID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, "ALTER TABLE trakrf.locations ENABLE TRIGGER maintain_location_path")
	require.NoError(t, err)

	// Sanity check: descendants query misses the legacy child (the bug).
	descBefore, err := store.GetDescendants(ctx, orgID, parent.ID)
	require.NoError(t, err)
	descIDsBefore := map[int]bool{}
	for _, l := range descBefore {
		descIDsBefore[l.ID] = true
	}
	assert.False(t, descIDsBefore[childID], "legacy non-canonical child should be invisible before recompute")

	// Run the recompute.
	var rows int
	err = pool.QueryRow(ctx, "SELECT trakrf.recompute_location_paths()").Scan(&rows)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, rows, 1, "at least the legacy child should be updated")

	// Child path is now canonical.
	child, err := store.GetLocationByID(ctx, orgID, childID)
	require.NoError(t, err)
	require.NotNil(t, child)
	assert.Equal(t, "whs_01.whs_07_03", child.Path)

	// Descendants query now finds the child.
	descAfter, err := store.GetDescendants(ctx, orgID, parent.ID)
	require.NoError(t, err)
	descIDsAfter := map[int]bool{}
	for _, l := range descAfter {
		descIDsAfter[l.ID] = true
	}
	assert.True(t, descIDsAfter[childID], "child must appear in descendants after recompute")

	// Ancestors query from the child finds the parent.
	ancestors, err := store.GetAncestors(ctx, orgID, childID)
	require.NoError(t, err)
	ancIDs := map[int]bool{}
	for _, l := range ancestors {
		ancIDs[l.ID] = true
	}
	assert.True(t, ancIDs[parent.ID], "parent must appear in ancestors after recompute")

	// Recompute is idempotent on the now-canonical state.
	err = pool.QueryRow(ctx, "SELECT trakrf.recompute_location_paths()").Scan(&rows)
	require.NoError(t, err)
	assert.Equal(t, 0, rows, "second recompute must be a no-op")
}
```

- [ ] **Step 1.2: Run the new tests, confirm they all fail**

Run: `just backend test-integration -run 'TestUpdateLocation_Reparent|TestUpdateLocation_ChangeExternalKey|TestRecomputeLocationPaths' ./internal/storage/...`

Expected: all four FAIL. The cascade tests fail because `c.Path` stays `a.b.c` (no cascade trigger). The recompute tests fail with `function trakrf.recompute_location_paths() does not exist`.

If `TestUpdateLocation_Reparent_CascadesDescendants` passes — investigate; the cascade may already exist and the spec may be wrong.

- [ ] **Step 1.3: Commit failing tests**

```bash
git add backend/internal/storage/locations_path_cascade_integration_test.go
git commit -m "test(locations): TRA-577 add failing tests for path cascade + recompute"
```

---

## Task 2: Write the migration

**Files:**
- Create: `backend/migrations/000038_location_path_backfill_and_cascade.up.sql`
- Create: `backend/migrations/000038_location_path_backfill_and_cascade.down.sql`

- [ ] **Step 2.1: Write the up migration**

Write to `backend/migrations/000038_location_path_backfill_and_cascade.up.sql`:

```sql
SET search_path = trakrf,public;

-- ============================================================================
-- TRA-577 — location.path backfill + descendant cascade
-- See docs/superpowers/specs/2026-05-02-tra-577-location-path-cascade-design.md
--
-- Two artifacts:
--  1. cascade_location_path()  — AFTER UPDATE trigger that walks descendants
--                                when a row's path changes (re-parent or
--                                external_key rename).
--  2. recompute_location_paths() — idempotent walker that rewrites every row's
--                                path to the canonical form. Called once here
--                                to fix legacy rows; future migrations that
--                                touch path semantics call it the same way.
-- ============================================================================

-- 1. Cascade trigger function. Slices the OLD prefix off each descendant's
--    path and prepends NEW.path. AFTER timing → NEW.path is fully populated
--    by the existing BEFORE trigger (update_location_path) before we run.
CREATE OR REPLACE FUNCTION trakrf.cascade_location_path() RETURNS TRIGGER AS $$
BEGIN
    -- Cascade writes only `path`, not parent_location_id or external_key, so
    -- neither the BEFORE trigger nor this AFTER trigger re-fires on these
    -- updates. No recursion.
    UPDATE trakrf.locations
    SET path = NEW.path || subpath(path, nlevel(OLD.path))
    WHERE path <@ OLD.path
      AND id != NEW.id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS cascade_location_path_change ON trakrf.locations;
CREATE TRIGGER cascade_location_path_change
    AFTER UPDATE OF parent_location_id, external_key
    ON trakrf.locations
    FOR EACH ROW
    WHEN (OLD.path IS DISTINCT FROM NEW.path)
    EXECUTE FUNCTION trakrf.cascade_location_path();

-- 2. Idempotent canonical-path recompute. Walks the tree from roots and
--    rewrites any row whose path differs from its canonical value. Returns
--    the number of rows updated so callers (and tests) can detect drift.
--
--    Includes soft-deleted rows: path is part of the row regardless of state
--    and the GiST index covers them, so divergence between live and deleted
--    rows would surface again later.
CREATE OR REPLACE FUNCTION trakrf.recompute_location_paths() RETURNS INT AS $$
DECLARE
    rows_updated INT;
BEGIN
    WITH RECURSIVE canonical AS (
        SELECT id, parent_location_id,
               text2ltree(replace(lower(external_key), '-', '_')) AS new_path
        FROM trakrf.locations
        WHERE parent_location_id IS NULL

        UNION ALL

        SELECT l.id, l.parent_location_id,
               c.new_path || text2ltree(replace(lower(l.external_key), '-', '_'))
        FROM trakrf.locations l
        JOIN canonical c ON l.parent_location_id = c.id
    )
    UPDATE trakrf.locations l
    SET path = c.new_path
    FROM canonical c
    WHERE l.id = c.id
      AND l.path IS DISTINCT FROM c.new_path;

    GET DIAGNOSTICS rows_updated = ROW_COUNT;
    RETURN rows_updated;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION trakrf.recompute_location_paths() IS
    'Rewrites locations.path to canonical form (lower(replace(external_key, ''-'', ''_''))). Idempotent. See backend/migrations/CONVENTIONS.md.';

-- 3. One-time backfill for legacy rows that predate the canonical rule.
SELECT trakrf.recompute_location_paths();
```

- [ ] **Step 2.2: Write the down migration**

Write to `backend/migrations/000038_location_path_backfill_and_cascade.down.sql`:

```sql
SET search_path = trakrf,public;

-- TRA-577 down: drop the cascade trigger and both functions. We do NOT
-- restore the pre-backfill (non-canonical) path values — that data is gone
-- and is not needed for any rollback scenario.
DROP TRIGGER IF EXISTS cascade_location_path_change ON trakrf.locations;
DROP FUNCTION IF EXISTS trakrf.cascade_location_path();
DROP FUNCTION IF EXISTS trakrf.recompute_location_paths();
```

- [ ] **Step 2.3: Run the integration tests, confirm all four pass**

Run: `just backend test-integration -run 'TestUpdateLocation_Reparent|TestUpdateLocation_ChangeExternalKey|TestRecomputeLocationPaths' ./internal/storage/...`

Expected: all four PASS.

If `TestUpdateLocation_ChangeExternalKey_CascadesDescendants` fails with `assert.Equal(t, "a.b2", updated.Path)` returning `"a.b2"` but `c.Path = "a.b.c"` — verify the WHEN clause: `OLD.path IS DISTINCT FROM NEW.path` should be true because the BEFORE trigger rewrote path. If it isn't, suspect that the BEFORE trigger isn't firing on external_key (it should — column-list includes it).

- [ ] **Step 2.4: Run the full locations integration suite to confirm no regressions**

Run: `just backend test-integration ./internal/storage/...`

Expected: all PASS.

- [ ] **Step 2.5: Commit migration**

```bash
git add backend/migrations/000038_location_path_backfill_and_cascade.up.sql \
        backend/migrations/000038_location_path_backfill_and_cascade.down.sql
git commit -m "feat(db): TRA-577 cascade trigger + path recompute migration"
```

---

## Task 3: Add the conventions doc

**Files:**
- Create: `backend/migrations/CONVENTIONS.md`

- [ ] **Step 3.1: Write the conventions file**

Write to `backend/migrations/CONVENTIONS.md`:

```markdown
# Migration Conventions

## Path-derived columns

When a migration changes `update_location_path()`, `cascade_location_path()`,
or anything else that derives `locations.path` from `external_key` / parent,
call `SELECT trakrf.recompute_location_paths();` from the same migration.

The triggers only re-fire on column-scoped INSERT/UPDATE events, so existing
rows do not pick up new derivation rules without an explicit recompute.
`recompute_location_paths()` is idempotent — safe to call against an
already-canonical table.

History: TRA-577 introduced this convention after BB15 found 704 of 713
preview-DB rows had non-canonical paths because the canonical rule changed
in 000036 without a recompute.
```

- [ ] **Step 3.2: Commit**

```bash
git add backend/migrations/CONVENTIONS.md
git commit -m "docs(migrations): TRA-577 path-recompute convention"
```

---

## Task 4: Verify against preview reproduction

This step is verification only — no code changes. Confirms the migration would actually fix the BB15 finding when it ships.

- [ ] **Step 4.1: Apply both functions to a preview-snapshot equivalent**

Run the migration's body against preview using a transaction so we can roll back without changing preview:

```bash
psql "$PG_URL_PREVIEW" <<'SQL'
BEGIN;
SET search_path = trakrf,public;

-- Paste the cascade function + recompute function definitions from the up
-- migration (everything except the final SELECT).
\i backend/migrations/000038_location_path_backfill_and_cascade.up.sql

-- Verify BB15 D-2 reproduction case is fixed.
SELECT id, external_key, path
FROM trakrf.locations
WHERE external_key IN ('WHS-01','WHS-07-03')
ORDER BY path;

-- Spot-check overall canonicalization.
SELECT COUNT(*) AS total,
       COUNT(*) FILTER (
         WHERE path::text != lower(replace(path::text, '-', '_'))
       ) AS still_non_canonical
FROM trakrf.locations
WHERE deleted_at IS NULL;

ROLLBACK;
SQL
```

Expected output:
- WHS-07-03 row's path is `whs_01.whs_07_03` (or similar canonical form, depending on its actual external_key after preview-side edits).
- `still_non_canonical = 0` (all rows canonical after recompute).

If `still_non_canonical > 0`, investigate before continuing — the recompute is missing rows.

---

## Task 5: Validate, push, open PR

- [ ] **Step 5.1: Run lint**

Run: `just lint`

Expected: clean. (The Go test file uses standard patterns; SQL migrations are not linted.)

- [ ] **Step 5.2: Re-run integration suite as a final check**

Run: `just backend test-integration ./internal/storage/...`

Expected: all PASS.

- [ ] **Step 5.3: Push branch**

Run: `git push -u origin fix/tra-577-location-path-backfill-cascade`

- [ ] **Step 5.4: Open PR**

```bash
gh pr create --title "fix(db): TRA-577 location.path backfill + descendant cascade" --body "$(cat <<'EOF'
## Summary

Closes [TRA-577](https://linear.app/trakrf/issue/TRA-577).

- Adds AFTER UPDATE cascade trigger so re-parenting (or renaming `external_key` on) a location updates descendant `path` values atomically.
- Adds idempotent `trakrf.recompute_location_paths()` and calls it once from the migration to fix the 704/713 preview-DB rows that hold non-canonical paths from the pre-000036 rule.
- Codifies the recompute-on-rule-change convention in `backend/migrations/CONVENTIONS.md` so this divergence doesn't recur.

Spec: `docs/superpowers/specs/2026-05-02-tra-577-location-path-cascade-design.md`

## Test plan

- [x] `just backend test-integration ./internal/storage/...` — four new integration tests cover re-parent cascade, external_key cascade, recompute idempotency, and legacy-row repair.
- [ ] After preview deploy: spot-check `SELECT path FROM trakrf.locations WHERE external_key IN ('WHS-01','WHS-07-03');` — both should be canonical.
- [ ] After preview deploy: `GET /api/v1/locations/<WHS-01-id>/descendants` returns the WHS-07-03 child (BB15 D-2 reproduction case).

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 5.5: Capture PR URL** for the user.

---

## Self-review notes

Spec coverage: cascade trigger (Task 2), recompute function (Task 2), migration call (Task 2), four-test coverage (Task 1), conventions doc (Task 3), preview verification (Task 4). All acceptance bullets covered.

Placeholder scan: clean — no TBD/TODO, every step has the actual code or command.

Type consistency: `mkLoc` returns `*location.Location`; `UpdateLocation` returns `*location.LocationWithParent`. Both have a `Path` field — confirmed via `backend/internal/models/location/location.go`.
