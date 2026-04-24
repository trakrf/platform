# TRA-482 — Enforce `UNIQUE(org_id, type, value)` on identifiers

**Ticket:** [TRA-482](https://linear.app/trakrf/issue/TRA-482/same-duplicate-identifier-bug-pattern-on-identifiers-table)
**Parent pattern:** [TRA-475](https://linear.app/trakrf/issue/TRA-475/critical-post-with-duplicate-identifier-creates-duplicates-instead-of) (assets + locations)
**Priority:** High
**Status:** design

## Problem

`POST /api/v1/assets/{identifier}/identifiers` and `POST /api/v1/locations/{identifier}/identifiers` accept duplicate tag values and create additional rows instead of returning `409 Conflict`.

Root cause is identical to TRA-475, on a different table. The `trakrf.identifiers` table declares:

```sql
UNIQUE(org_id, type, value, valid_from)
```

…and `valid_from` defaults to `CURRENT_TIMESTAMP` per insert. Every row has a unique timestamp, so `(org_id, type, value)` collisions are not actually prevented. Duplicate tag identifiers can accumulate on different assets.

Audit of environments (2026-04-24):

- **Cloud (prod):** 29 live identifiers, **0 duplicates**.
- **Preview:** 47 live identifiers, **1 duplicate group** — `(org=217329607, type=rfid, value=10021)` attached to both asset `ASSET-0003` (Sladget) and `ASSET-0004` (Squidget). Confirmed by user to be stale test data from the bug itself.

## Root cause

Same shape as TRA-475. The defeated constraint is a table-level `UNIQUE(org_id, type, value, valid_from)` on `identifiers` (migration `000009_identifiers.up.sql`). Handler-side the INSERT omits `valid_from`, so Postgres populates it with `CURRENT_TIMESTAMP`, guaranteeing uniqueness that was never the intent.

The HTTP handlers (`handlers/assets/assets.go:486` `AddIdentifier`, `handlers/locations/locations.go:665` `AddIdentifier`) already map the storage-layer "already exist" error to 409 Conflict. They will work correctly the moment the constraint starts firing.

The storage-layer error parser (`storage/identifiers.go:221` `parseIdentifierError`) switches on `pgErr.ConstraintName` and currently only recognizes the legacy name `identifiers_org_id_type_value_valid_from_key`. Under the new partial unique index the constraint name changes, so the specific-message branch stops matching — but the "duplicate key" substring fallback at line 231 keeps the 409 mapping alive. Still, the switch should be updated so the specific-case message path stays lit.

## Approach

### Constraint shape: partial unique index

```sql
CREATE UNIQUE INDEX identifiers_org_id_type_value_unique
  ON trakrf.identifiers (org_id, type, value)
  WHERE deleted_at IS NULL;
```

- Matches TRA-475's partial-index pattern on assets/locations.
- Preserves soft-delete semantics: a soft-deleted identifier can be recreated (new surrogate id) — e.g. a tag that was detached then re-attached to a different asset continues to work.
- Removes `valid_from` from uniqueness, ending the bug.

### Dedup of existing duplicates: soft-delete losers

Same pattern as TRA-475. Before installing the index, soft-delete all but the most-recently-updated live row per `(org_id, type, value)`:

```sql
WITH ranked AS (
  SELECT id,
         row_number() OVER (
           PARTITION BY org_id, type, value
           ORDER BY updated_at DESC, id DESC
         ) AS rn
  FROM trakrf.identifiers
  WHERE deleted_at IS NULL
)
UPDATE trakrf.identifiers i
   SET deleted_at = now()
  FROM ranked r
 WHERE i.id = r.id AND r.rn > 1;
```

- `updated_at DESC, id DESC` tie-break: deterministic, keeps the most recent user intent.
- Idempotent: zero rows updated if no duplicates exist (cloud case).
- Preview impact: identifier row `1337879127` (attached to Squidget, ASSET-0004) wins; row `1443649481` (attached to Sladget, ASSET-0003) gets soft-deleted. Sladget loses its RFID 10021 attachment. Confirmed acceptable by user — stale test data.

### Cross-table uniqueness (no change, but worth locking in a test)

The schema-level `identifier_target` CHECK constraint already says an identifier row attaches to *exactly one* of `asset_id` or `location_id`. The new partial index on `(org_id, type, value)` spans the whole table — so a tag value attached to an asset blocks the same value being attached to a location in the same org. This is the right behavior (a physical RFID tag can't be on both a pallet and a shelf at the same time) and deserves a test to prevent regression.

### Handler behavior (unchanged)

- Handlers continue to map "already exist" errors to 409 Conflict. Error envelope unchanged.
- `valid_from` handling is untouched (TRA-468 territory).

### Storage fix: `parseIdentifierError` switch

Update `backend/internal/storage/identifiers.go:221`:

```go
switch pgErr.ConstraintName {
case "identifiers_org_id_type_value_unique":
    return fmt.Errorf("identifier %s:%s already exists", identifierType, value)
case "identifier_target":
    return fmt.Errorf("identifier must be linked to exactly one asset or location")
}
```

The legacy constraint name is dropped — it won't be emitted after the migration runs. The "duplicate key" substring fallback at line 231 remains as defense-in-depth.

No other storage changes required. Attach paths (`AddIdentifierToAsset`, `AddIdentifierToLocation`) are plain INSERTs that already route errors through `parseIdentifierError`.

## Down migration

Mirror of TRA-475's down:

1. `DROP INDEX IF EXISTS trakrf.identifiers_org_id_type_value_unique;`
2. Re-add the legacy `UNIQUE(org_id, type, value, valid_from)` constraint.

Soft-deleted rows from the dedup step are not reverted. Re-running up is idempotent.

## Testing

Integration tests, matching TRA-475's coverage shape.

**Rewrite existing misnamed tests** — they currently assert 201 but are named `_Returns409`:

1. `TestAssetsAddIdentifier_DuplicateValue_Returns409` (`assets_integration_test.go:454`) — drop the `valid_from='2000-01-01'` seed dance. Seed asset B with identifier via handler, POST same value to attach to asset A → **409**.
2. `TestLocationsAddIdentifier_DuplicateValue_Returns409` (`locations/integration_test.go:245`) — same rewrite, locations.

**Add new tests**:

3. Reuse-after-soft-delete on asset attach: attach identifier, soft-delete it, attach same value again → 201. (Proves `WHERE deleted_at IS NULL` clause is live.)
4. Reuse-after-soft-delete on location attach: same, locations.
5. Cross-table collision: attach value to an asset, attempt same value on a location in the same org → 409.

Migration test (embed_test.go / harness equivalent) is not added — TRA-475 did not add one for assets/locations, and the integration tests above cover the post-migration state.

## Out of scope

- Changing identifier-attachment semantics (single-owner vs. shared).
- Tag-value reuse policy after hard-delete (the partial index handles soft-delete reuse; hard-delete isn't implemented).
- Touching `valid_from` / `valid_to` temporal-validity columns (TRA-468 territory).
- Ripping out `valid_from` from inserts — handler continues to let the DB default fire.

## Acceptance criteria (from ticket)

- [ ] Audit production for existing `(org_id, type, value)` duplicates on `identifiers`. *(Done in design phase — cloud clean, preview has 1 stale test row.)*
- [ ] Apply migration that deduplicates existing live duplicates (most-recently-updated wins).
- [ ] Drop `UNIQUE(org_id, type, value, valid_from)`; add `UNIQUE(org_id, type, value) WHERE deleted_at IS NULL`.
- [ ] POST `/api/v1/assets/{identifier}/identifiers` returns 409 on duplicate tag value.
- [ ] POST `/api/v1/locations/{identifier}/identifiers` returns 409 on duplicate tag value.
- [ ] Integration tests for duplicate rejection on both attach paths.
