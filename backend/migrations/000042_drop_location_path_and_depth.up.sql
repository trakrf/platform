SET search_path = trakrf,public;

-- ============================================================================
-- TRA-684 — drop locations.path (ltree) and locations.depth (generated)
--
-- BB29 F9 / C3: tree_path was a materialized-path denormalization derived from
-- external_key (lower, hyphens → underscores). It silently allowed
-- case-collisions (LOSSY-CASE and lossy-case folded to the same path) and
-- appeared prominently enough on responses to invite misuse as a join key.
-- Hierarchy size at TrakRF's scale (3–5 levels, low thousands of rows) makes
-- recursive CTE on the parent_location_id btree index cheap enough that the
-- denormalization solves a problem we don't have.
--
-- Drop order: triggers → trigger functions → indexes → generated column →
-- ltree column → recompute helper. ltree extension is left installed; nothing
-- else depends on it but a future feature might.
-- ============================================================================

-- 1. AFTER cascade trigger and BEFORE maintain trigger.
DROP TRIGGER IF EXISTS cascade_location_path_change ON trakrf.locations;
DROP TRIGGER IF EXISTS maintain_location_path ON trakrf.locations;

-- 2. Trigger functions.
DROP FUNCTION IF EXISTS trakrf.cascade_location_path();
DROP FUNCTION IF EXISTS trakrf.update_location_path();
DROP FUNCTION IF EXISTS trakrf.recompute_location_paths();

-- 3. Indexes that referenced path / depth.
DROP INDEX IF EXISTS trakrf.locations_path_gist_idx;
DROP INDEX IF EXISTS trakrf.locations_depth_idx;

-- 4. Generated `depth` column (declared GENERATED ALWAYS AS nlevel(path));
--    must drop before path because of the dependency.
ALTER TABLE trakrf.locations DROP COLUMN IF EXISTS depth;

-- 5. ltree path column.
ALTER TABLE trakrf.locations DROP COLUMN IF EXISTS path;
