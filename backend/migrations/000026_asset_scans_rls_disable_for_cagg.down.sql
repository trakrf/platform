-- Reverse of 000026: restore RLS on asset_scans. Reaching this down-migration
-- means 000028 (which also re-enables RLS) and 000027 have already been rolled
-- back, so asset_scans is currently RLS-disabled; put it back to the TRA-875
-- baseline.
ALTER TABLE trakrf.asset_scans ENABLE ROW LEVEL SECURITY;
