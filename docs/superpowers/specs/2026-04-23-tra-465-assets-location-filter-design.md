# TRA-465 — Fix `/assets?location=X` filter and hydration

**Status:** Design
**Ticket:** [TRA-465](https://linear.app/trakrf/issue/TRA-465)
**Related:** [TRA-210](https://linear.app/trakrf/issue/TRA-210) (parent), [TRA-392](https://linear.app/trakrf/issue/TRA-392) (API design)

## Problem

`GET /api/v1/assets?location=WHS-01` returns `total_count: 0` even when assets exist at WHS-01. The same filter on `/api/v1/locations/current?location=WHS-01` correctly returns those assets.

The assets list handler *is* wired for a `location` filter: `backend/internal/handlers/assets/assets.go:373` populates `ListFilter.LocationIdentifiers`, and `backend/internal/storage/assets.go:798` turns that into `WHERE l.identifier = ANY($n::text[])`. The filter runs, but it filters against a stale source.

Root cause: `assets.current_location_id` is a dead denormalized column. It is written only on explicit create / update (`storage/assets.go:16–55`, `87–139`, `312–327`) and is never kept in sync from incoming scans. The actual "current location" of every asset lives in `asset_scans` — which is exactly what `/locations/current` derives from via a `DISTINCT ON (asset_id) ... ORDER BY asset_id, timestamp DESC` CTE (`storage/reports.go`).

So the assets filter runs correctly against a column that does not reflect reality. Assets whose initial `current_location_id` happened to match the filter value get returned; everyone else (the overwhelming majority in any steady-state system) does not.

## Goals

- `GET /api/v1/assets?location={identifier}` returns assets whose **most recent scan** is at that location.
- Multiple `?location=` params OR together, matching existing docs.
- Response hydration of `current_location` (the JSON field on `PublicAssetView` — sourced from `AssetWithLocation.CurrentLocationIdentifier` internally) reflects the same scan-derived truth as the filter — no internal disagreement.
- Integration test coverage guards against the exact regression we're fixing.

## Non-goals

- Historical / ever-at-location filtering.
- Location hierarchy filtering (assets at X or any descendant).
- Removing the `current_location_id` column or its write paths — deferred to a follow-up (see below).
- Continuous aggregate / Timescale `last()`-based implementation — deferred.

## Design

### Storage layer change — `ListAssetsFiltered` / `CountAssetsFiltered`

Replace the `LEFT JOIN trakrf.locations l ON l.id = a.current_location_id` with a `LEFT JOIN` through a `latest_scans` CTE, mirroring the pattern in `storage/reports.go`:

```sql
WITH latest_scans AS (
    SELECT DISTINCT ON (s.asset_id)
        s.asset_id,
        s.location_id
    FROM trakrf.asset_scans s
    WHERE s.org_id = $1
    ORDER BY s.asset_id, s.timestamp DESC
)
SELECT
    a.id, a.org_id, a.identifier, a.name, a.type, a.description,
    ls.location_id AS current_location_id,
    a.valid_from, a.valid_to, a.metadata,
    a.is_active, a.created_at, a.updated_at, a.deleted_at,
    l.identifier
FROM trakrf.assets a
LEFT JOIN latest_scans ls ON ls.asset_id = a.id
LEFT JOIN trakrf.locations l
    ON l.id = ls.location_id
   AND l.org_id = a.org_id
   AND l.deleted_at IS NULL
WHERE <predicates>
ORDER BY <orderBy>
LIMIT $N OFFSET $N+1
```

Both the `current_location_id` surrogate returned to the caller and the hydrated `current_location_identifier` come from `latest_scans`. They cannot disagree with the filter.

`buildAssetsWhere` is unchanged — it already emits `l.identifier = ANY($n::text[])`, which now runs against the scan-derived `l` join. No change to multi-value handling.

`CountAssetsFiltered` receives the same CTE + join rewrite so that counts and rows agree.

**Assets with no scans.** They LEFT JOIN to `NULL` on `ls` → `NULL` on `l.identifier`. Unfiltered lists still include them with `current_location_identifier: null`. `?location=X` correctly excludes them (`NULL = ANY(...)` is never true).

**Soft-deleted locations.** The existing `AND l.deleted_at IS NULL` clause still applies; an asset whose latest scan is at a soft-deleted location appears in unfiltered lists with `current_location_identifier: null` and is excluded by any `?location=X` filter.

### Handler and filter struct

No changes. The filter struct field `LocationIdentifiers` already documents "OR semantics when multi-valued." Handler registration of the `location` filter key is already correct.

### Model / response shape

`AssetWithLocation.CurrentLocationIdentifier` already exists and is scanned from `l.identifier`. Its JSON contract is unchanged; only its *source* moves from the stale column to the scan-derived CTE. Same for `Asset.CurrentLocationID`.

This is a semantic change visible to callers who were relying on the stale value. In practice nothing currently relies on that — no internal consumer, and the public API contract has always been documented as "current location."

### Docs

Customer-facing docs for this filter live in the separate `trakrf-docs` repo at `docs/api/pagination-filtering-sorting.md`. That file already lists `location` (repeatable) as a filter on `GET /api/v1/assets` — the filter was documented, it just did not work. A small follow-up PR in `trakrf-docs` (opened **after** this platform PR merges and the behavior is live, per the "docs behind backend reality" convention) clarifies that the filter value is the location **identifier** (natural key). No docs change is needed in the platform repo itself; OpenAPI handler comments already describe the parameter correctly.

## Testing

Single integration test in `backend/internal/handlers/assets/assets_integration_test.go`, four cases:

1. **Happy path.** Asset with one scan at WHS-01 → returned by `?location=WHS-01`, `current_location_identifier` is `"WHS-01"`.
2. **OR semantics.** Two assets, one at WHS-01 and one at WHS-02 → `?location=WHS-01&location=WHS-02` returns both; `?location=WHS-01` returns only the first.
3. **Stale-column regression guard.** Asset created with `current_location_id` pointing at WHS-01 but latest scan at WHS-02 → **not** returned by `?location=WHS-01`, returned by `?location=WHS-02`. This is the test that would have caught the original bug.
4. **No-scans asset.** Asset with no scans at all → not returned by any `?location=...` filter value.

No unit-level tests for `buildAssetsWhere` — the change is in the surrounding query structure, not the predicate builder.

## Deferred follow-up work (post-v1, not a TeamCentral launch blocker)

Separate Linear ticket to be filed after this PR merges. The "latest scan per asset" CTE will become a bottleneck once scan volume is large enough to dominate every `/assets` query. Two Timescale-native options to evaluate:

- **Continuous aggregate** over `asset_scans` materializing latest scan per asset, refreshed on a policy.
- **`last()` hyperfunction** query (already present for reports — see `QueryEngineTimescaleLast` in `storage/reports.go`) wired through to the assets query engine as well.

Scope of the follow-up should also address: removing or deprecating the `current_location_id` physical column and its write paths, now that it is unused by reads.

## Files touched (platform repo)

- `backend/internal/storage/assets.go` — rewrite `ListAssetsFiltered`, `CountAssetsFiltered` queries.
- `backend/internal/handlers/assets/assets_integration_test.go` — new integration test with four cases above.

A separate, smaller PR in the `trakrf-docs` repo (opened after this one merges) clarifies the `location` filter value in `docs/api/pagination-filtering-sorting.md`.

## Risks

- **Query cost.** Every `/assets` list now computes `latest_scans` over all `asset_scans` rows in the org. At MVP scale this is fine; the follow-up replaces it before scale-out.
- **Behavior change for callers.** Any caller that was relying on the stale `current_location_identifier` value will now see the scan-derived value. This is a correctness improvement, but it is observable. Call out in PR description.
