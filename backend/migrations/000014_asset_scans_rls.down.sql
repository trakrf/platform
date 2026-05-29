-- TRA-875 — revert RLS policy on asset_scans.

SET search_path = trakrf, public;

DROP POLICY IF EXISTS org_isolation_asset_scans ON asset_scans;

ALTER TABLE asset_scans DISABLE ROW LEVEL SECURITY;
