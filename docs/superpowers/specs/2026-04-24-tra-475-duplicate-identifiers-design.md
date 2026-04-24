# TRA-475 — Enforce `UNIQUE(org_id, identifier)` on assets and locations

**Ticket:** [TRA-475](https://linear.app/trakrf/issue/TRA-475/critical-post-with-duplicate-identifier-creates-duplicates-instead-of)
**Follow-up:** [TRA-482](https://linear.app/trakrf/issue/TRA-482/same-duplicate-identifier-bug-pattern-on-identifiers-table) (same pattern on `identifiers`)
**Priority:** Urgent
**Status:** design

## Problem

`POST /api/v1/assets` and `POST /api/v1/locations` accept duplicate identifiers and create additional rows instead of returning `409 Conflict`.

Evidence (black-box evaluation #6, 2026-04-24):

```
POST /api/v1/assets {"identifier":"ASSET-BB8-1777044453","name":"BB8 eval asset"}
→ 201 (surrogate_id: 190540959)

POST /api/v1/assets {"identifier":"ASSET-BB8-1777044453","name":"dup"}
→ 201 (surrogate_id: 422006167)
```

Same behavior on `/locations`. The docs explicitly promise:

1. `Errors → Idempotency` — "retrying with the same identifier hits the UNIQUE(org_id, identifier) constraint and returns 409"
2. `CHANGELOG v1.0.0` — "POST /api/v1/locations now returns 409 conflict (not 500) on duplicate identifiers"
3. `Resource identifiers` — "Clients should key on identifier"

Every public-API integration that retries on network wobble (which the docs explicitly recommend) will silently accumulate duplicates. TeamCentral, the first public-API customer, syncs assets from an ERP — this is a data-integrity regression visible on every retry.

## Root cause

Both tables declare:

```sql
UNIQUE(org_id, identifier, valid_from)
```

Handlers force `valid_from = time.Now().UTC()` on every POST (`handlers/assets/assets.go:83-86`, `handlers/locations/locations.go:106-109`). Every insert has a unique timestamp, so the constraint never fires. Storage-layer error parsing (`strings.Contains(err.Error(), "duplicate key")`) works correctly — it simply never sees a conflict.

Secondary issue found in audit: `storage.BatchCreateAssets` (`storage/assets.go:316`) uses `ON CONFLICT (org_id, identifier) DO UPDATE SET deleted_at = NULL, …`. No constraint on `(org_id, identifier)` exists — this path raises "no matching unique index" at runtime under conflict. Must be fixed as part of this change so the new partial index is reachable.

Other identifier-lookup paths (`GetAssetByIdentifier`, `GetLocationByIdentifier`, `CheckDuplicateIdentifiers`, `reports.go` joins) already filter `deleted_at IS NULL` and remain correct.

## Approach

### Constraint shape: partial unique index

```sql
CREATE UNIQUE INDEX assets_org_id_identifier_unique
  ON trakrf.assets (org_id, identifier)
  WHERE deleted_at IS NULL;
```

- Matches the docs' stated `UNIQUE(org_id, identifier)` promise for live rows.
- Preserves the existing soft-delete semantic: a soft-deleted identifier can be re-created (new surrogate_id), so `DELETE → POST same identifier → 201` continues to work. Docs' idempotency claim (`second DELETE returns 404`) still holds.
- Removes `valid_from` from uniqueness, ending the original bug class on these two tables.

### Dedup of existing duplicates: soft-delete losers

Before installing the new index, the migration identifies duplicates and soft-deletes all but the most-recently-updated live row per `(org_id, identifier)`:

```sql
WITH ranked AS (
  SELECT id,
         row_number() OVER (
           PARTITION BY org_id, identifier
           ORDER BY updated_at DESC, id DESC
         ) AS rn
  FROM trakrf.assets
  WHERE deleted_at IS NULL
)
UPDATE trakrf.assets a
   SET deleted_at = now()
  FROM ranked r
 WHERE a.id = r.id AND r.rn > 1;
```

- Soft-delete (not hard): consistent with the rest of the app, reversible if a customer flags a wrong winner, keeps audit trail.
- `updated_at DESC, id DESC` as the tiebreak: if two rows have identical `updated_at`, the larger surrogate id (= later insert) wins. Deterministic.
- Idempotent: if no duplicates exist, zero rows updated.
- Losing rows keep their FK relationships (`identifiers.asset_id`, `asset_scans.asset_id`). Those histories stay intact; only the identifier lookup path now always lands on the winner.

Production impact: one customer, ~17 assets total. Very small blast radius. If any dedup decision is wrong, the losing row can be manually restored by clearing `deleted_at`.

### Handler behavior (unchanged)

- Handlers continue to set `valid_from = now()` when the caller omits it. Redundant with the DB default (`CURRENT_TIMESTAMP`), but harmless and recently touched by TRA-468 — no reason to stir it.
- The existing `strings.Contains(err.Error(), "already exist")` → `409 Conflict` branch in both `assets.go` and `locations.go` already returns the documented error envelope. Once the constraint fires, the response shape is correct.

### Storage fix: `BatchCreateAssets` ON CONFLICT

Change:

```go
ON CONFLICT (org_id, identifier) DO UPDATE SET
    name = EXCLUDED.name,
    …
    deleted_at = NULL,
    updated_at = NOW()
```

to:

```go
ON CONFLICT (org_id, identifier) WHERE deleted_at IS NULL DO UPDATE SET
    name = EXCLUDED.name,
    …
    updated_at = NOW()
```

- The `WHERE` predicate is required for Postgres to match the partial index in `ON CONFLICT`.
- `deleted_at = NULL` is dropped from the DO UPDATE: under the partial index, conflicts only match live rows, so the line is dead code. Batch-importing a row whose identifier matches only a soft-deleted row now inserts a **new** live row rather than reviving the deleted one. This is a behavior change for bulk import, called out in the commit message.

No other storage changes required. All other reads/lookups already filter `deleted_at IS NULL`.

## Down migration

Reverses cleanly:

1. `DROP INDEX assets_org_id_identifier_unique;` and likewise for locations.
2. Re-add the legacy `UNIQUE(org_id, identifier, valid_from)` constraint on both tables.

Soft-deleted rows created by the dedup step remain soft-deleted — no-op on down. If a re-migration is needed, the dedup step is idempotent.

## Testing

Integration tests (real DB, existing harness in `backend/internal/handlers/{assets,locations}/public_write_integration_test.go`):

1. POST duplicate asset identifier → 409 Conflict with documented envelope.
2. POST duplicate location identifier → 409 Conflict with documented envelope.
3. POST asset → DELETE → POST same identifier → 201 (re-use after soft-delete works).
4. POST → DELETE → DELETE → expect 201, 204, 404 (lock in docs' DELETE-idempotency claim).
5. PUT asset rename into another live asset's identifier → 409.
6. POST location update path, same rename-collision → 409.

Migration test (in `backend/migrations/embed_test.go` or a new integration test):

7. Seed two live rows with the same `(org_id, identifier)`. Run migration. Assert: the row with the larger `updated_at` remains live; the loser has `deleted_at IS NOT NULL`; index exists.

## Out of scope

- `identifiers` table uniqueness — filed as TRA-482. Different dedup semantics (shared-owner concerns), not a copy/paste.
- Ripping out `valid_from`/`valid_to` temporal validity columns — post-v1 YAGNI cleanup, separate ticket.
- Upsert semantics (PUT-as-create) — explicit out-of-scope in TRA-475.
- Bulk-import upsert behavior review — out of scope; this change fixes the syntax so the existing behavior keeps working under the new index, nothing more.

## Acceptance criteria (from ticket)

- [x] Partial UNIQUE index on `(org_id, identifier) WHERE deleted_at IS NULL` enforced on `assets` and `locations`.
- [x] POST with duplicate identifier returns 409 Conflict with documented envelope.
- [x] Migration deduplicates existing duplicates (keep most-recently-updated, soft-delete others).
- [x] Integration tests for duplicate rejection on both resources.
- [x] DELETE idempotency claim (`second DELETE returns 404`) verified by integration test.
