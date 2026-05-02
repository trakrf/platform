# TRA-577 — `location.path` backfill + descendant cascade

**Linear:** [TRA-577](https://linear.app/trakrf/issue/TRA-577) (parent: TRA-575 BB15 launch readiness)
**Status:** Design
**Author:** Claude (Opus 4.7)
**Date:** 2026-05-02

## Problem

`locations.path` (Postgres `ltree`) is computed by `update_location_path()` (a BEFORE INSERT/UPDATE trigger introduced in 000018, rewritten in 000036 to use the renamed `external_key` column). Two distinct defects:

1. **Legacy paths are not canonical.** The trigger only re-fires when `parent_location_id` or `external_key` changes. Rows that predate the canonical rule still hold their old preserved-literal paths, so GiST filters such as `path LIKE 'whs_01.%'` miss them and `/locations/{id}/ancestors|descendants` returns wrong (often empty) sets. **Verified in preview**: 704 of 713 location rows have non-canonical paths, including the BB15 reproduction case `WHS-01 (path=whs_01)` → `WHS-07-03 (path=WHS-01.WHS-07-03)`.
2. **No descendant cascade on re-parent.** `parent_location_id` is writable on `PUT /api/v1/locations/{id}`. The BEFORE trigger fixes the row being updated but does not walk its descendants. After re-parenting `B` (under `A`) to live under `D`, `B.path` becomes `d.b` ✓ but `C.path` is still `a.b.c` ✗. Subsequent `<@`/`@>` queries return wrong sets and `ORDER BY path` returns wrong tree order.

Documenting "don't re-parent" as a v1 constraint is rejected — too easy to trip, especially for the first public-API customer.

## Goals

- Every existing row in `trakrf.locations` ends up with `path = canonical_path(external_key)` joined recursively with parent's canonical path.
- Future re-parent or external_key changes cascade to descendants atomically inside the same statement.
- Migration is idempotent — safe to re-run on already-correct rows (so the convention also covers future migrations that touch path semantics).
- Cost of cascade stays bounded by the GiST index on `path`.
- A short convention doc tells future migration authors what to do when they touch `update_location_path()` or anything that derives `path`.

## Non-goals

- Changing the canonical-path rule itself (it is correct; legacy data just doesn't match).
- Renaming `path` → `tree_path` for the public schema (handled by C-1 in TRA-580).
- Moving path computation to the application layer (it's a database concern by design — single source of truth for both BEFORE-write computation and cascade).
- Recomputing each descendant's external_key portion during cascade (descendants' own canonical labels remain stable; only the prefix changes — slicing is sufficient).

## Design

### 1. Backfill function (extracted, called from migration)

Introduce `trakrf.recompute_location_paths()` — a no-arg PL/pgSQL function that walks the `locations` table from roots and rewrites every row's `path` to canonical form. The migration calls it once. Future migrations that change path semantics call it too. Tests call it to assert idempotency.

```sql
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
```

**Notes**

- Includes soft-deleted rows. Path is part of the row regardless of soft-delete state, and the GiST index covers them. Keeping all rows canonical avoids future "this looked fine until we un-deleted it" surprises.
- Returns row count so test assertions can prove idempotency on the second call (`assert.Equal(0, n)`).
- Updates `path` directly. The existing BEFORE trigger fires on `UPDATE OF parent_location_id, external_key` only, so direct path UPDATEs do not re-fire it. The new AFTER cascade trigger (below) is similarly column-scoped — no recursion.
- Does not touch `updated_at` because the column-list trigger guard would not fire and the existing `update_locations_updated_at` trigger would. Acceptable: backfill is a system-maintenance event and an updated_at touch on every legacy row is fine.

### 2. AFTER cascade trigger

Pair the existing BEFORE trigger with an AFTER trigger that walks descendants when the row's path actually changed.

```sql
CREATE OR REPLACE FUNCTION trakrf.cascade_location_path() RETURNS TRIGGER AS $$
BEGIN
    UPDATE trakrf.locations
    SET path = NEW.path || subpath(path, nlevel(OLD.path))
    WHERE path <@ OLD.path
      AND id != NEW.id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cascade_location_path_change
    AFTER UPDATE OF parent_location_id, external_key
    ON trakrf.locations
    FOR EACH ROW
    WHEN (OLD.path IS DISTINCT FROM NEW.path)
    EXECUTE FUNCTION trakrf.cascade_location_path();
```

**Why this is safe and bounded**

- AFTER timing → `NEW.path` is fully computed by the BEFORE trigger before cascade runs.
- `WHEN (OLD.path IS DISTINCT FROM NEW.path)` short-circuits no-op updates (a PUT that touches another field but leaves parent and external_key alone won't trip the WHEN clause anyway, but if external_key is set to its own value, this is a cheap belt).
- The cascade `UPDATE` writes only `path`. Neither trigger fires on `path` changes (BEFORE: `OF parent_location_id, external_key`; AFTER: same). So **no recursion**.
- `path <@ OLD.path AND id != NEW.id` selects strict descendants. The GiST index makes the scan an indexed range over the subtree.
- Org scoping not required at trigger level because `path` is constructed per org from `external_key` (org-unique). Two orgs can both have `path='whs_01'` and a re-parent in one org will not match the other org's subtree under `<@` because their roots differ.
- `subpath(path, nlevel(OLD.path))` slices off the descendant's old prefix; prepending `NEW.path` yields the new full path. Works for both re-parent and external_key changes.

### 3. Migration call site

The single new migration:

1. Creates the cascade function and trigger.
2. Creates `trakrf.recompute_location_paths()`.
3. Calls `SELECT trakrf.recompute_location_paths();` once.

Order matters: cascade trigger first so any concurrent re-parent during deploy is also handled, but in practice migrations run in a transaction with no concurrent traffic — order is mostly cosmetic.

### 4. Conventions doc

New file `backend/migrations/CONVENTIONS.md` with a single section initially:

> **Path-derived columns.** When a migration changes `update_location_path()`, `cascade_location_path()`, or anything else that derives `locations.path`, call `SELECT trakrf.recompute_location_paths();` from the same migration. The trigger only re-fires on column-scoped INSERT/UPDATE events, so existing rows do not pick up new rules without an explicit recompute.

We will accrete other conventions here over time. Single-rule starter file is fine.

### 5. Tests

All tests live under `backend/internal/storage/` and are tagged `//go:build integration` (real-DB tests via `testutil.SetupTestDB`).

- **`TestRecomputeLocationPaths_Idempotent`** — Build a small tree under the canonical rule. Call `recompute_location_paths()`. Assert returned row count is 0. (No rows changed because canonical already matched.)
- **`TestRecomputeLocationPaths_FixesLegacyRows`** — Insert a row with the trigger temporarily disabled, manually setting a non-canonical `path` (e.g., `WHS-01.WHS-07-03`). Re-enable the trigger, call `recompute_location_paths()`, assert the row's new path is canonical (`whs_01.whs_07_03`). Assert ancestors/descendants queries return correct sets after the recompute.
- **`TestUpdateLocation_Reparent_CascadesDescendants`** — Build A→B→C. Create D as a sibling of A (or under A). Re-parent B to live under D via `UpdateLocation`. Assert `B.path = d.b` and `C.path = d.b.c`. Assert `GetDescendants(D)` returns {B, C}.
- **`TestUpdateLocation_ChangeExternalKey_CascadesDescendants`** — Build A→B→C. Update B's external_key to `b2`. Assert `B.path = a.b2` and `C.path = a.b2.c`.

## Risks / open questions

- **Performance of cascade on deep subtrees.** Bounded by GiST + per-org subtree size. v1 customer subtrees are small (single warehouses with bays/shelves). Acceptable.
- **Concurrent re-parent vs. backfill in production.** Migration runs at deploy time during low traffic; transaction-isolated. If a PUT lands mid-migration, it will simply re-trigger the cascade on its own row.
- **Idempotency under future schema drift.** If we add a column whose value participates in path derivation later (we won't — but if), the `IS DISTINCT FROM` guard ensures the recompute is still safe to call repeatedly.

## Acceptance (mirrors the ticket)

- [ ] Migration adds cascade trigger + recompute function and calls the recompute once.
- [ ] Trigger walks descendants on `OF parent_location_id, external_key` updates when `path` changes.
- [ ] Integration tests cover re-parent cascade, external_key cascade, idempotency, and legacy-row fix.
- [ ] `/locations/{id}/ancestors` and `/descendants` return correct sets for the BB15 reproduction (WHS-01 → WHS-07-03) after the migration runs.
- [ ] `backend/migrations/CONVENTIONS.md` documents the recompute-on-rule-change rule.
