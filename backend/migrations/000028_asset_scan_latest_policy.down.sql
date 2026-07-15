-- Reverse of 000028: drop the refresh policy and re-disable RLS so 000027 can
-- drop the CAGG (and 000026's down then restores RLS to the baseline).
SELECT remove_continuous_aggregate_policy('trakrf.asset_scan_latest');

ALTER TABLE trakrf.asset_scans DISABLE ROW LEVEL SECURITY;
