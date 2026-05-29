# TRA-875 — RLS policy on `trakrf.asset_scans`

**Date:** 2026-05-29
**Branch:** `feat/tra-875-asset-scans-rls`
**Ticket:** [TRA-875](https://linear.app/trakrf/issue/TRA-875)
**Origin:** TRA-865 root-cause analysis surfaced that `asset_scans` is the only
tenant table without RLS.

## Decision

**Outcome (A): add the RLS policy.** Enable row-level security on
`trakrf.asset_scans` with the same `org_isolation_*` policy shape used by the
other six tenant tables. Keep the explicit `WHERE org_id = $n` filters in every
query as defense-in-depth (no query-logic churn).

This makes `asset_scans` consistent with `assets`, `locations`, `tags`,
`scan_points`, `scan_devices`, and `bulk_import_jobs` — every tenant table is
now RLS-protected, with no exceptions to reason about.

## Why (A) and not (B)

The ticket framed this as: if the current no-RLS posture was a deliberate
TimescaleDB/perf trade-off, document it (B); if it was "we just didn't add it,"
add it (A). The investigation found the latter.

### The stated rationale was a misconception

The TRA-720 clean-schema spec records the reason for omitting RLS on
`asset_scans` as simply *"No RLS on hypertables"*
(`docs/superpowers/specs/2026-05-26-tra-720-clean-schema-stack-design.md`,
lines 112 and 256). That is factually wrong: **TimescaleDB enforces RLS on
hypertables.** A policy on the hypertable parent applies to every query routed
through the parent. The well-known limitation
([timescaledb#7830](https://github.com/timescale/timescaledb/issues/7830)) is
that policies are *not propagated to the chunks themselves* — a role with
**direct** `_timescaledb_internal` chunk access can bypass the parent policy.

That bypass is not reachable in our deployment: the runtime `trakrf-app-<env>`
role is granted CRUD only on tables in schema `trakrf`
(`migrations/README.md`), not on `_timescaledb_internal`. All app queries go
through the `trakrf.asset_scans` parent, where the policy is enforced.

So the original exclusion was a categorical "hypertables can't have RLS" belief,
not a measured trade-off. Outcome (A).

### The cost is genuinely small

Every existing `asset_scans` query is already org-scoped and `WithOrgTx`-wrapped:

| Method | File | `WithOrgTx` | `WHERE org_id` |
|---|---|---|---|
| `ListCurrentLocations` | `storage/reports.go` | yes | yes |
| `CountCurrentLocations` | `storage/reports.go` | yes | yes |
| `ListAssetHistory` | `storage/reports.go` | yes | yes |
| `CountAssetHistory` | `storage/reports.go` | yes | yes |
| `CountActiveAssetsAtLocation` | `storage/locations.go` | yes | yes |
| `SaveInventoryScans` (INSERT) | `storage/inventory.go` | yes | yes (org_id param) |

Because the GUC `app.current_org_id` is already set inside every one of these,
turning RLS on is a no-op for the happy path. RLS only changes behaviour for the
failure path: a future query that forgets `WithOrgTx` now produces a loud
500 (`22P02`/`42704`, like TRA-865) instead of silently returning another org's
rows. Loud beats silent.

### The ingestion path is not a blocker

`process_tag_scans()` (the AFTER-INSERT trigger on `tag_scans`) auto-inserts into
`locations`, `scan_devices`, `scan_points`, `assets`, and `tags` — **all five are
already RLS-protected** — before it inserts into `asset_scans`. A `USING`-only
policy is also applied as the `WITH CHECK` for INSERTs. So whatever role and
context make the trigger's writes to the five RLS tables succeed today will
equally cover its write to `asset_scans`. Adding RLS to `asset_scans` introduces
**no new failure mode** in ingestion — it simply joins the set of tables the
trigger already writes under RLS. (The external MQTT→`tag_scans` connector lives
outside this repo; like the TRA-810 FDW pull, bulk/ingestion paths run with
`BYPASSRLS` or `row_security = OFF`.)

## TimescaleDB caveats (documented, not blocking)

These are genuine RLS-on-hypertable limitations. None apply today, but they
constrain future Timescale-native work on `asset_scans`:

1. **Compression:** RLS is not supported on *compressed* chunks. `asset_scans`
   has no compression policy today. If one is added later, the RLS interaction
   must be re-evaluated (it may require `BYPASSRLS` background workers or
   excluding compressed chunks from RLS-enforced reads).
2. **Continuous aggregates:** RLS on the underlying hypertable does **not**
   extend to a continuous aggregate built on it. `asset_scans` has no CAGG today.
   Any future CAGG must carry its own `org_id` column + filtering (or its own
   policy); it will not inherit `asset_scans`' isolation.
3. **OrderedAppend:** RLS disables the OrderedAppend chunk-ordering optimization,
   so time-ordered scans (`ORDER BY timestamp DESC`, `DISTINCT ON`) may add a
   sort. Negligible at current volume (prod has tens of `asset_scans` rows
   pre-launch); the explicit `WHERE org_id = $n` keeps the planner well-fed
   regardless. Re-benchmark if scan volume grows materially.

## Implementation

1. **Migration `000014_asset_scans_rls`** (up + down, per the 000011+ convention):
   ```sql
   ALTER TABLE asset_scans ENABLE ROW LEVEL SECURITY;
   CREATE POLICY org_isolation_asset_scans ON asset_scans
       USING (org_id = current_setting('app.current_org_id')::BIGINT);
   ```
   Down drops the policy and disables RLS. No `FORCE ROW LEVEL SECURITY` —
   matches the six existing policies (the owning `trakrf-migrate` role is exempt;
   the `trakrf-app` role is enforced).

2. **Integration test** `storage/asset_scans_rls_integration_test.go`
   (analog to TRA-874's `TestTestAppRole_RLSIsEnforced`): seed an org + asset +
   `asset_scan` via the admin pool; assert the RLS-enforced app pool (a) cannot
   read the scan with no org context, (b) can read it under the owning org's
   context, and (c) cannot see it under a different org's context.

3. **Comment fix:** `CountAssetHistory` in `storage/reports.go` carried a comment
   asserting `asset_scans` "carries no RLS policy today" — updated to reflect the
   policy now exists.

## Verification

- `just backend migrate` applies cleanly; down migration reverts.
- New integration test passes against the RLS-enforced test role.
- Full backend test suite green (no existing test regressed — all `asset_scans`
  queries already set org context).

## Consequences

- `asset_scans` is no longer the lone exception in the tenant-isolation story;
  `check-rls-guard` (which already covers `reports.go`, `inventory.go`,
  `locations.go`) is now the sufficient guard for it, same as the other tables.
- The explicit `WHERE org_id` filters are retained as redundant defense-in-depth
  and to keep the query planner informed; they were not removed.
- Future compression / continuous-aggregate work on `asset_scans` must account
  for the caveats above.
