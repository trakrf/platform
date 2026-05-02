SET search_path = trakrf,public;

-- ============================================================================
-- TRA-577 — location.path backfill + descendant cascade
-- See docs/superpowers/specs/2026-05-02-tra-577-location-path-cascade-design.md
--
-- Two artifacts:
--  1. cascade_location_path()    — AFTER UPDATE trigger that walks
--                                  descendants when a row's path changes
--                                  (re-parent or external_key rename).
--  2. recompute_location_paths() — idempotent walker that rewrites every
--                                  row's path to the canonical form. Called
--                                  once here to fix legacy rows; future
--                                  migrations that touch path semantics call
--                                  it the same way (see CONVENTIONS.md).
-- ============================================================================

-- 1. Cascade trigger function. Slices the OLD prefix off each descendant's
--    path and prepends NEW.path. AFTER timing → NEW.path is fully populated
--    by the existing BEFORE trigger (update_location_path) before we run.
--    The cascade UPDATE writes only `path`, not parent_location_id or
--    external_key, so neither the BEFORE trigger nor this AFTER trigger
--    re-fires on these updates. No recursion.
CREATE OR REPLACE FUNCTION trakrf.cascade_location_path() RETURNS TRIGGER AS $$
BEGIN
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
--    Includes soft-deleted rows: path is part of the row regardless of
--    state and the GiST index covers them, so divergence between live and
--    deleted rows would surface again later.
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
