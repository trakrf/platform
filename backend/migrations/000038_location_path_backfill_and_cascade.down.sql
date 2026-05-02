SET search_path = trakrf,public;

-- TRA-577 down: drop the cascade trigger and both functions. We do NOT
-- restore the pre-backfill (non-canonical) path values — that data is gone
-- and is not needed for any rollback scenario.
DROP TRIGGER IF EXISTS cascade_location_path_change ON trakrf.locations;
DROP FUNCTION IF EXISTS trakrf.cascade_location_path();
DROP FUNCTION IF EXISTS trakrf.recompute_location_paths();
