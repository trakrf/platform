-- TRA-1022 (3 of 3): re-enable RLS on asset_scans (restoring the TRA-875
-- baseline now that the CAGG exists) and register the refresh policy.
--
-- Both statements are transaction-safe (cf. add_retention_policy in 000008), so
-- they share one file. add_continuous_aggregate_policy just registers a
-- background job; it works with RLS on. The refresh runs as the CAGG owner and
-- bypasses asset_scans RLS.
--
-- Offsets target ~30-60s staleness with a 1-minute bucket: refresh every 30s,
-- materialize completed buckets up to 1 minute behind now, scanning only the
-- last 3 hours for invalidations each run (late-arriving scans beyond that are
-- not expected). Tune against preview timing under load.
ALTER TABLE trakrf.asset_scans ENABLE ROW LEVEL SECURITY;

SELECT add_continuous_aggregate_policy('trakrf.asset_scan_latest',
    start_offset      => INTERVAL '3 hours',
    end_offset        => INTERVAL '1 minute',
    schedule_interval => INTERVAL '30 seconds');
