# TRA-512 — `/locations/current` deleted-asset semantics

**Linear:** [TRA-512](https://linear.app/trakrf/issue/TRA-512/locationscurrent-can-return-identifiers-no-longer-in-assets-document)
**Date:** 2026-04-26
**Status:** Approved for implementation

## Summary

`/api/v1/locations/current` is a derived view computed from immutable scan history. It currently joins `trakrf.assets` without filtering `deleted_at IS NULL`, so it returns rows for soft-deleted assets — assets that no longer surface from `/assets`. Naive integrations that render those identifiers display ghost rows.

This spec ships both halves of TRA-512 in one platform PR:

- **AC1 (docs):** rewrite the OpenAPI `@Description` to explain the underlying scan-history model and the new opt-in flag.
- **AC2 (behavior, originally post-v1):** option 3 — elide deleted assets by default; opt back in via `?include_deleted=true`. Promoted out of post-v1 because we are already in this code path.

A second, follow-up PR in `trakrf-docs` will verify the Redocusaurus rebuild picks up the new spec.

## Behavior

### Default request: `GET /api/v1/locations/current`

- Deleted assets are excluded from results.
- `total_count` reflects elided rows.
- Response items omit `asset_deleted_at` (the field is `*time.Time` with `omitempty`, so live rows have no key at all).

### Opt-in: `?include_deleted=true`

- Live and deleted assets both return.
- Deleted rows populate `asset_deleted_at` with the deletion timestamp.
- Live rows still omit the field.

### `?include_deleted=false`

Same as default.

### Invalid value (e.g. `?include_deleted=banana`)

400 Bad Request via the existing list-param error envelope. Use `httputil.ListAllowlist.BoolFilters` (strict `true`/`false` only — `1`/`0`/`TRUE` are rejected, matching project convention).

### Edge cases

- **Hard-deleted asset:** the `JOIN trakrf.assets` drops the row regardless of the flag. Soft-delete is the only mode affected.
- **q-search with default elision:** `q` matches against `a.name`, `a.identifier`, and active-identifier values. After the change, the `a.deleted_at IS NULL` predicate excludes the entire row, so q-search consistently follows the elision default. This is the desired outcome.
- **Pagination consistency:** count and list use the same predicate, so paging is stable.

## API surface

### `@Description` (option B from brainstorm)

> Snapshot of each asset's most recent location, filterable by natural key. Because this view is derived from immutable scan history, it can resolve identifiers for assets that have since been deleted. By default those rows are excluded; pass `include_deleted=true` to include them, and check `asset_deleted_at` to distinguish deleted from live.

### New query parameter

```
@Param include_deleted query bool false "include rows for soft-deleted assets" default(false)
```

### Response item shape

`PublicCurrentLocationItem` is a lean projection (just natural-key identifiers, no surrogates). After change:

```json
{
  "asset": "FORK-007",
  "location": "BAY-3",
  "last_seen": "2026-04-25T18:33:00Z",
  "asset_deleted_at": "2026-04-20T14:00:00Z"
}
```

Live rows omit `asset_deleted_at` entirely.

## Implementation surface

| File | Change |
|---|---|
| `backend/internal/models/report/current_location.go` | Add `IncludeDeleted bool` to `CurrentLocationFilter`; add `AssetDeletedAt *time.Time` to `CurrentLocationItem`. |
| `backend/internal/models/report/public.go` | Add `AssetDeletedAt *time.Time` (`json:"asset_deleted_at,omitempty"`) to `PublicCurrentLocationItem`; update `ToPublicCurrentLocationItem`. |
| `backend/internal/storage/reports.go` | Both query builders (`buildCurrentLocationsQueryDistinctOn`, `buildCurrentLocationsQueryTimescale`) and `CountCurrentLocations`: add `a.deleted_at` to projection, add `(a.deleted_at IS NULL OR $N::bool)` predicate (param appended at end). |
| `backend/internal/handlers/reports/current_locations.go` | Add `include_deleted` to `httputil.ListAllowlist.Filters` and `BoolFilters`; read `params.Filters["include_deleted"]` and set `filter.IncludeDeleted = vs[0] == "true"`. Update `@Description` and add `@Param include_deleted`. |
| `docs/api/openapi.public.{yaml,json}` | Regenerated via `just backend api-spec`. |
| Storage integration tests | Default elides deleted; opt-in returns deleted with populated `AssetDeletedAt`; q-search consistent with default. Cover both query engines. |
| Handler tests | Param parsing happy paths + invalid value → 400. |
| `public_integration_test.go` | One end-to-end assertion path for `?include_deleted=true`. |
| `collections_empty_test.go` | **Must update.** `TestListCurrentLocations_EmptyReturnsNonNil` mocks the exact column list and positional args — add `asset_deleted_at` to the row schema and the new `IncludeDeleted` to `WithArgs`. |

### SQL change pattern

Append `IncludeDeleted` as last positional param. Predicate added inside the WHERE block of both list queries and the count query:

```sql
AND (a.deleted_at IS NULL OR $N::bool)
```

`$N::bool` evaluates to `false` for the default request (predicate enforces `IS NULL`) and `true` for opt-in (predicate satisfied unconditionally).

## Out of scope

- **Frontend / internal UI.** The web app consumes a different code path (`CurrentLocationItem` directly, not `PublicCurrentLocationItem`). No frontend change in this PR.
- **Other endpoints with similar deleted-asset semantics.** Ticket scopes to `/locations/current`.
- **Redocusaurus / docs-site sync.** Separate PR in `trakrf-docs` after this one merges/deploys, per "ship docs behind backend reality."

## Verification

Before claiming done:

- `just backend test` — backend suite green (storage integration, handler, public integration, RLS sentinel).
- `just backend api-spec` — regenerates `openapi.public.yaml`; commit the diff.
- `just backend api-lint` — spec lints clean (Redocly).
- Manual `curl` against dev server: default request shows no deleted rows; `?include_deleted=true` shows them with timestamp.

## Linear follow-ups

- AC2 decision: option 3 (elision by default), shipping in this PR. Comment on TRA-512 noting that AC2 is no longer deferred to post-v1.
