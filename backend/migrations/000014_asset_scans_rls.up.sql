-- TRA-875 — RLS policy on asset_scans.
--
-- asset_scans was the only tenant table without row-level security. The
-- TRA-720 clean-schema rebuild excluded it under the belief that hypertables
-- can't carry RLS ("No RLS on hypertables"). That is a misconception:
-- TimescaleDB enforces RLS at the hypertable parent for any query routed
-- through it. The known limitation (timescaledb#7830) is that policies are not
-- propagated to chunks — but the runtime trakrf-app role has no access to
-- _timescaledb_internal, so every app query goes through this parent where the
-- policy applies.
--
-- Every asset_scans query is already WithOrgTx-wrapped with an explicit
-- WHERE org_id, so enabling RLS is a no-op on the happy path. It only changes
-- the failure path: a future query that forgets WithOrgTx now fails loud
-- (22P02/42704, like TRA-865) instead of silently leaking another org's rows.
--
-- USING-only policy (no WITH CHECK, no FORCE) — identical shape to the other
-- six tenant tables; the USING qual doubles as the INSERT WITH CHECK.
--
-- Caveats for future Timescale-native work on this table (none apply today):
--   * RLS is unsupported on COMPRESSED chunks — no compression policy here yet.
--   * RLS does not extend to continuous aggregates — any CAGG must filter org_id
--     itself.
--   * RLS disables the OrderedAppend optimization — negligible at current volume.

SET search_path = trakrf, public;

ALTER TABLE asset_scans ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_asset_scans ON asset_scans
    USING (org_id = current_setting('app.current_org_id')::BIGINT);
