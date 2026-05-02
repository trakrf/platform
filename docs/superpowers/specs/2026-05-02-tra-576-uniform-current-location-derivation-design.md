# TRA-576 — Uniform `current_location_*` derivation across asset read paths

## Problem

`GET /api/v1/assets/{id}` and `GET /api/v1/assets/lookup` return
`current_location_id: null` while `GET /api/v1/assets` returns the populated
id for the same asset. This breaks the FK-pair invariant that
`PublicAssetView` commits to: `current_location_id` and
`current_location_external_key` must be both populated or both null.

BB15 reproduction (asset id `790505327`, external_key `ASSET-0001`):

| endpoint                           | id          | external_key |
| ---------------------------------- | ----------- | ------------ |
| `GET /assets?limit=2`              | `542787020` | `WHS-01`     |
| `GET /assets/790505327`            | `null`      | `WHS-01`     |
| `GET /assets/lookup?external_key=` | `null`      | `WHS-01`     |

## Root cause

Two independent divergences in `backend/internal/storage/assets.go`:

1. **Different precedence.** `ListAssetsFiltered` (lines 721–744) uses
   `COALESCE(ls.location_id, a.current_location_id)` (scan-first).
   `getAssetWithLocationByID` (lines 505–525) and `GetAssetByExternalKey`
   (lines 603–627) use `COALESCE(a.current_location_id, ls.location_id)`
   (FK-first).

2. **Different source for the two fields.** Detail/lookup queries
   `SELECT a.current_location_id` (raw column) but the JOIN against
   `locations` uses `COALESCE(a.current_location_id, ls.location_id)` to
   resolve `l.external_key`. So `current_location_id` is the raw column
   (often null) while `current_location_external_key` is from the
   coalesced location row. Result: `(null, "WHS-01")`.

The list path got both fields from the same expression by writing the
coalesced int back into `a.CurrentLocationID` (line 732). Detail/lookup
never aligned the SELECT and the JOIN.

## Decision

Unify on **list-path semantics** across all three read paths:

- precedence: `COALESCE(ls.location_id, a.current_location_id)` (scan-first,
  FK-fallback)
- both `current_location_id` and `current_location_external_key` derived
  from the same expression in SELECT and JOIN

This matches the ticket's directive ("matching whatever the list path
does") and the TrakRF record-of-origin philosophy stated in the ticket
("`current_location_*` is always derived, never stored on the asset row").

The TRA-477 comments in `getAssetWithLocationByID` / `GetAssetByExternalKey`
that prefer the explicit FK become stale and are removed. The integer
field that gets returned will be whatever `l.id` matches in the JOIN, so
both fields are guaranteed to either come from the same row or be null
together.

## Scope

Change the SQL in two functions:

- `Storage.getAssetWithLocationByID` (used by `GET /{id}`, plus Create and
  Update responses — fix lands there as a free side effect)
- `Storage.GetAssetByExternalKey` (used by `GET /lookup`)

`ListAssetsFiltered` already has the correct shape; no change.

## Test plan

New integration test file `backend/internal/handlers/assets/current_location_consistency_integration_test.go`:

1. **Both populated** — create asset, insert an `asset_scans` row pointing
   at a known location, fetch via list / detail / lookup, assert all three
   return identical non-nil `(current_location_id, current_location_external_key)`.
2. **Both null** — create asset with no FK and no scan, fetch via three
   endpoints, assert all three return `(null, null)`.
3. **FK-only fallback** — create asset with `current_location_id` set but
   no scan, fetch via three endpoints, assert FK pair populated and
   identical across endpoints (covers TRA-495 fallback).

Existing unit tests in `models/asset/public_test.go` are unaffected.

## Out of scope

- The four `.tra555-needs-rewrite` test files. They're already tagged for
  separate rework; not adding to that pile.
- The bulk-import path (POST /assets/bulk) — out of ticket scope.
- Schema/spec changes to `PublicAssetView` — already correct.

## Acceptance mapping

- AC1 (consistent FK pair across three reads) — covered by all three test
  cases.
- AC2 (BB15 reproduction case identical across reads) — covered by the
  scan-with-location test case (mirrors the BB15 setup).
- AC3 (integration test for the invariant) — new test file.
- AC4 (`PublicAssetView` schema unchanged) — no schema edits.
