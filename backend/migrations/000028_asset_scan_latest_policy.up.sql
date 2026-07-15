-- TRA-1022 (3 of 3): re-enable RLS on asset_scans (restoring the TRA-875
-- baseline now that the CAGG exists) and register the refresh policy.
--
-- Both statements are transaction-safe (cf. add_retention_policy in 000008), so
-- they share one file. add_continuous_aggregate_policy just registers a
-- background job; it works with RLS on. The refresh runs as the CAGG owner and
-- bypasses asset_scans RLS.
--
-- start_offset => NULL is deliberate and load-bearing: the report needs each
-- asset's latest scan no matter how old, so the materialization must cover ALL
-- history. A bounded start_offset would leave assets last seen before the window
-- permanently unmaterialized (invisible to the report). Refreshes stay cheap
-- after the first run because Timescale only re-materializes invalidated ranges
-- (recent appends), not the whole range every time; the first automatic run (or
-- the post-deploy manual refresh — see the PR) does the one-time backfill.
--
-- end_offset => 1 minute / schedule 30s targets ~1 minute staleness with a
-- 1-minute bucket. To tighten toward 30s, lower end_offset below the bucket so
-- the current partial bucket is re-materialized each run. Tune on preview.
ALTER TABLE trakrf.asset_scans ENABLE ROW LEVEL SECURITY;

SELECT add_continuous_aggregate_policy('trakrf.asset_scan_latest',
    start_offset      => NULL,
    end_offset        => INTERVAL '1 minute',
    schedule_interval => INTERVAL '30 seconds');
