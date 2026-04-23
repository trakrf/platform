# TRA-468 — Normalize `valid_from` / `valid_to` across the API

**Linear:** [TRA-468](https://linear.app/trakrf/issue/TRA-468/normalize-valid-fromvalid-to-three-conventions-for-no-end-date-in-one)
**Related:** TRA-447 (API create-path defaults — already fixed the inbound side)
**Date:** 2026-04-23
**Status:** Approved — ready for implementation plan

## Problem

Black-box evaluation #5 (2026-04-23) found three different conventions for "no end date" in a single `GET /api/v1/locations` response:

| Location   | `valid_from`            | `valid_to`              | Convention            |
|------------|-------------------------|-------------------------|-----------------------|
| WHS-01     | `0001-01-01T00:00:00Z`  | `0001-01-01T00:00:00Z`  | Go zero-time          |
| Assets     | real date               | `2099-12-31T...`        | Far-future sentinel   |
| WHS-07-03  | (omitted)               | (omitted)               | Null / absent         |

Three client branches to handle "no end date" in one resource list. The zero-time records predate TRA-447's inbound defaults; the `2099-12-31` sentinel is a legacy pattern from earlier seed data. TRA-447 fixed the *create* path so new records are clean; TRA-468 finishes the job by backfilling old rows, auditing update paths, and locking in a single JSON response shape.

## Convention (the one true answer)

- **`valid_from`** — `TIMESTAMPTZ NOT NULL`. Defaults to `now()` on insert. **Always present** in API responses as RFC3339. Every row has a meaningful effective-date (at latest, its creation moment); treating `null` as "no constraint" here would be a lie.
- **`valid_to`** — `TIMESTAMPTZ NULL`. Defaults to `NULL`. **`NULL` = "no expiry"** and is **omitted** from API JSON responses (via `omitempty` on `*time.Time`). Clients see either an RFC3339 timestamp or no `valid_to` key at all — never a sentinel.
- **Zero-time (`0001-01-01`)** must never appear on the wire or on disk.
- **Far-future sentinels (`2099-12-31`)** must never appear on the wire or on disk.

## Scope

All four tables that carry these columns — even ones not currently on the public API surface — so future promotion doesn't resurface the bug:

- `organizations`
- `assets`
- `locations`
- `identifiers`

## Out of scope (explicit)

- `valid_from < valid_to` validation (per ticket).
- DB `CHECK` constraints forbidding sentinel ranges. Application-level guards (handler defaults + `FlexibleDate` zero→nil coercion) are sufficient; a DB CHECK would tax every fixture, seed, and import for marginal gain. Reconsider if a future regression shows it's needed.
- `FlexibleDate` parsing or serialization behavior — unchanged.
- Any UI work.
- `created_at` / `updated_at` / `deleted_at` conventions — different pattern (audit timestamps, always present), not in the bug.

## Approach

### 1. Data-cleanup migration

One new migration under `backend/migrations/` — `NNNN_normalize_valid_dates.{up,down}.sql`. Single transaction, four tables:

```sql
-- For each of: organizations, assets, locations, identifiers
UPDATE <t>
   SET valid_from = created_at
 WHERE valid_from < '1900-01-01';

UPDATE <t>
   SET valid_to = NULL
 WHERE valid_to IS NOT NULL
   AND (valid_to < '1900-01-01' OR valid_to > '2099-01-01');
```

Thresholds are range-based so any yet-unseen sentinel (e.g., `1970-01-01` unix-epoch, `9999-12-31`) gets swept up alongside the two known ones. No legitimate business data lives outside `[1900-01-01, 2099-01-01)`.

**Down migration:** no-op with a comment explaining that destroyed sentinels cannot be reconstructed and TRA-468 is one-way data cleanup. The rest of the schema is unchanged, so rollback of the migration sequence won't break the DB — it just can't restore the bad data.

**No schema column changes.** Existing nullability and defaults already match the target convention:
- `valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP` ✓
- `valid_to   TIMESTAMPTZ DEFAULT NULL` ✓

### 2. Code audit (prevent regression)

TRA-447 fixed the create path. Still to verify and fix in this PR if needed:

**Update handlers** — `backend/internal/handlers/{assets,locations,organizations,identifiers}/`
- Confirm PATCH/PUT paths don't overwrite with zero-time when the client omits the field. `nil *time.Time` must mean "don't change"; a zero-valued pointer must not land in SQL.

**Storage layer** — `backend/internal/storage/*.go`
- Audit `mapReqToFields`-style helpers across all four resources. A nil `*time.Time` stays `NULL`; a zero `time.Time` is rejected before writing (either by omission or by coercion to `nil` at the request-DTO boundary via `FlexibleDate`).

**Response models** — `backend/internal/models/*/public.go`
- Confirm every public response struct declares `ValidTo *time.Time` with `json:"valid_to,omitempty"`. (Research indicates this is already true for assets, locations, organizations; will verify identifiers.)
- Confirm `ValidFrom` is `time.Time` (non-pointer) so it always serializes and never carries the zero-time surprise for properly-created rows.

### 3. Tests

**Integration tests — assertion-first (TDD).**

Two new tests (or additions to existing suites) under the integration layer:

1. **Omit `valid_to` on create → GET omits `valid_to` key.**
   `POST /api/v1/assets` (and `/locations`) with no `valid_to` → response JSON has no `valid_to` key at the top level. `GET` returns the same shape. Asserts the "omit when null" rule.

2. **Send `valid_to` on create → GET returns RFC3339 `valid_to`.**
   Same endpoints with an explicit `valid_to` → response contains `valid_to` as RFC3339 and GET returns it unchanged. Asserts the "present when set" rule.

**Migration regression test.**

One targeted test (under whatever integration gate the existing migration tests use):
- Seed rows directly via SQL with `valid_from = '0001-01-01'` and `valid_to = '2099-12-31'`.
- Run the migration.
- Assert the row now has `valid_from = created_at` and `valid_to IS NULL`.

Write the assertions first, watch them fail against current data / code state, then layer in the migration + any audit fixes and watch them pass.

### 4. Documentation (follow-up PR, after backend merges)

Per project rule "ship docs behind backend reality," docs land in a separate PR in the `trakrf-docs` repo after this backend PR merges and preview-deploys. That PR adds a short "Date fields" section to the API reference:

- `valid_from` — always present, RFC3339, UTC. Marks when the record became effective (defaults to creation time on insert).
- `valid_to` — **omitted when the record has no expiry.** Present as RFC3339 when set.
- Inbound: `FlexibleDate` accepts several formats (`YYYY-MM-DD`, `MM/DD/YYYY`, RFC3339, …); outbound is always RFC3339.

Noted as a follow-up in Linear; not touched in this branch.

## Files expected to change

- **New:** `backend/migrations/NNNN_normalize_valid_dates.up.sql` + `.down.sql`
- **Audit + potential fixes:**
  - `backend/internal/handlers/assets/assets.go`
  - `backend/internal/handlers/locations/locations.go`
  - `backend/internal/handlers/organizations/*.go`
  - `backend/internal/handlers/identifiers/*.go` (storage-layer audit even if no public handler exists today — the migration touches the table so the code path must stay clean)
  - `backend/internal/storage/{assets,locations,organizations,identifiers}.go`
  - `backend/internal/models/{asset,location,organization,identifier}/public.go`
- **Tests:** additions to the existing public integration test suites + one migration regression test.

## Definition of done

- Single convention for "no constraint" dates across all four resources.
- Migration backfills existing rows (range-based, both zero-time and far-future sentinels).
- Create + update + list handlers + storage paths write only the sanctioned shape.
- Integration tests assert the response shape on create/get for both "with valid_to" and "without valid_to".
- Migration regression test proves the backfill on representative sentinels.
- Follow-up docs ticket / PR queued (not required to close this ticket).
