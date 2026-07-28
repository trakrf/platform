-- TRA-1023 down: drop the dwell walk-back covering index.
--
-- The schema qualifier is _timescaledb_internal, not trakrf: CREATE INDEX on a
-- continuous aggregate is forwarded to the materialization hypertable, which
-- lives in _timescaledb_internal, and the index is created there under the name
-- given in the .up.sql. `DROP INDEX trakrf.asset_scan_latest_asset_bucket_loc_idx`
-- would report "does not exist" and silently leave the index behind.
DROP INDEX IF EXISTS _timescaledb_internal.asset_scan_latest_asset_bucket_loc_idx;
