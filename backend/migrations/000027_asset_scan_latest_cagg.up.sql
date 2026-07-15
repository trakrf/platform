-- TRA-1022 (2 of 3): continuous aggregate materializing latest-scan-per-asset-
-- per-bucket over asset_scans. The asset-locations report (and the
-- DELETE /locations/{id} guard) collapse these buckets with an outer
-- last()/max() grouped by asset_id, replacing the DISTINCT ON over asset_scans
-- that TRA-1021 had to defuse with SET LOCAL timescaledb.enable_skipscan=off.
--
-- MUST be the only statement in this file: CREATE MATERIALIZED VIEW ... WITH
-- (timescaledb.continuous) cannot run inside a transaction block, and
-- golang-migrate sends each migration file as a single Exec (a lone statement
-- auto-commits; >1 statement becomes one implicit transaction). RLS on
-- asset_scans was dropped in 000026 and is re-enabled in 000028.
--
-- Kept materialized_only (the default) — i.e. real-time aggregation is OFF.
-- TimescaleDB will not enable real-time aggregation on an RLS-guarded hypertable
-- either, and we keep asset_scans RLS (TRA-875). Reads therefore see only
-- materialized data and never touch the RLS base table; freshness is bounded by
-- the refresh policy in 000028 (~30-60s). A 1-minute bucket keeps that freshness
-- floor low. Tenant isolation on the CAGG is the explicit WHERE org_id = $N in
-- every read (org_id is carried in the GROUP BY); RLS does not extend to CAGGs.
CREATE MATERIALIZED VIEW trakrf.asset_scan_latest
WITH (timescaledb.continuous) AS
SELECT
    time_bucket(INTERVAL '1 minute', s.timestamp) AS bucket,
    s.org_id,
    s.asset_id,
    last(s.location_id, s.timestamp) AS location_id,
    max(s.timestamp)                 AS last_seen
FROM trakrf.asset_scans s
GROUP BY bucket, s.org_id, s.asset_id
WITH NO DATA;
