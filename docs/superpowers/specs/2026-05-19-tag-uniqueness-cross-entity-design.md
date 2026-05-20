# Tag Uniqueness Across Assets and Locations — Design

**Date:** 2026-05-19
**Status:** Approved, pending Linear ticket
**Related:** TRA-734 (asset location read-only), prior tag/identifier rename (migrations 000033/000036/000037)

## Problem

A single physical RFID tag (EPC) can only be attached to one entity at a
time — either an asset or a location, never both, and never to two assets
or two locations simultaneously. The current system silently allows
duplicate attachments at all three layers:

1. **Schema.** The `identifier_target` CHECK constraint enforces that a
   single tag *row* is asset-XOR-location, but `UNIQUE(org_id, type,
   value, valid_from)` is effectively a no-op for cross-row collisions
   because `valid_from` defaults to `CURRENT_TIMESTAMP` — two inserts of
   the same value get distinct keys.

2. **Backend.** `storage.AddTagToAsset` / `AddTagToLocation` plain-INSERT
   into `trakrf.tags`. The `parseTagError` 409 path for
   `tags_org_id_type_value_unique` exists but is unreachable in practice
   because the underlying constraint never fires.

3. **Frontend.** `lookupApi.byTag('rfid', epc)` fires only on barcode
   scan, not on typed input. The "Tag Already Assigned" confirmation
   modal's "Reassign" action just appends the value to the form's local
   tag list — clicking it doesn't detach from the old entity. On save,
   `addTag` fires on the new entity and the value ends up attached to
   **both**.

Net effect: the same EPC can attach to multiple entities simultaneously,
and "Reassign" creates a duplicate instead of moving the attachment.
Downstream, scan-event ingestion has to pick an owner ambiguously.

## Goals

- Make it impossible — at the storage layer — for two active tag rows in
  the same org to share `(type, value)`.
- Surface a typed, actionable error when a write would collide, naming
  the conflicting entity so the user can resolve it.
- Replace the misleading "Reassign" UX with inline error state and
  block-on-collision save semantics.

## Non-Goals

- An atomic reassign endpoint (`POST /api/v1/tags/reassign`). The
  workflow is explicit: remove from the old entity, then add to the new.
- Changes to scan-event ingestion's EPC → entity resolution. The new
  constraint makes resolution unambiguous as a side benefit; the
  ingestion code does not change.
- A "View conflicting entity" deep link from the inline error. The
  `external_key` ships in the 409 payload for future use, but the v1
  error is text-only.
- Re-enabling the currently-excluded `AssetForm` / `AssetFormModal`
  vitest files. Pre-existing exclusion, separate concern.

## Design

The work splits into three phases that share one ticket because each
prior phase makes the next one meaningful:

### Phase 1 — Schema migration

New numbered migration under `backend/migrations/`. Up SQL:

```sql
-- Step 1: Hard-stop guard. Refuse to create the constraint if any
-- collisions remain. Surfaces them in the error so the operator can
-- see what would have been clobbered.
DO $$
DECLARE
    dup_count INT;
BEGIN
    SELECT COUNT(*) INTO dup_count
    FROM (
        SELECT org_id, type, value
        FROM trakrf.tags
        WHERE deleted_at IS NULL
        GROUP BY org_id, type, value
        HAVING COUNT(*) > 1
    ) d;

    IF dup_count > 0 THEN
        RAISE EXCEPTION
          'cannot add tags_org_type_value_unique_active: % active (value, type, org) groups have duplicates. '
          'Resolve with: SELECT org_id, type, value, array_agg(id) FROM trakrf.tags '
          'WHERE deleted_at IS NULL GROUP BY org_id, type, value HAVING COUNT(*) > 1;',
          dup_count;
    END IF;
END $$;

-- Step 2: Partial unique index. WHERE deleted_at IS NULL is the only
-- predicate that's immutable; rely on app logic to keep valid_to /
-- is_active aligned. Soft-deleted history rows are free to repeat.
CREATE UNIQUE INDEX tags_org_type_value_unique_active
    ON trakrf.tags (org_id, type, value)
    WHERE deleted_at IS NULL;

-- Replace the old (org_id, type, value, valid_from) unique index that
-- was effectively no-op. Drop only after the partial index is in.
DROP INDEX IF EXISTS tags_org_id_type_value_unique;
```

Down SQL recreates the original index and drops the new one.

**Operator runbook** (in migration header comment):

```
Before applying in preview: soft-delete duplicate active rows with
  UPDATE trakrf.tags
     SET deleted_at = NOW()
   WHERE deleted_at IS NULL
     AND id NOT IN (
       SELECT DISTINCT ON (org_id, type, value) id
         FROM trakrf.tags
        WHERE deleted_at IS NULL
        ORDER BY org_id, type, value, created_at DESC
     );
Production: pre-launch, no real data; constraint expected to apply clean.
```

Four deliberate choices:

- **No baked-in auto-dedupe.** The migration refuses to clobber data
  silently — operator runs the cleanup explicitly. Preserves "no silent
  moves of physical tracking."
- **Partial index, not full UNIQUE.** Soft-deleted history rows
  (`deleted_at IS NOT NULL`) are free to repeat — this is how a tag's
  lineage survives a reassignment.
- **Soft-delete on cleanup, not hard DELETE.** Keeps the rows reachable
  for audit and matches the existing temporal model.
- **Drop the old useless index.** Avoids confusion from two overlapping
  uniqueness constraints; the new one is strictly stronger for active
  rows.

### Phase 2 — Backend service changes

**`storage.AddTagToAsset` / `AddTagToLocation`** (`backend/internal/storage/tags.go:84-128`):
wrap the INSERT in a transaction that does a pre-check inside the same
`WithOrgTx` so it's race-free:

```go
// inside WithOrgTx
var conflictAssetID, conflictLocationID *int
var conflictAssetName, conflictLocationName, conflictAssetExtKey, conflictLocationExtKey *string

err := tx.QueryRow(ctx, `
    SELECT t.asset_id, t.location_id,
           a.name, a.external_key,
           l.name, l.external_key
      FROM trakrf.tags t
      LEFT JOIN trakrf.assets    a ON a.id = t.asset_id
      LEFT JOIN trakrf.locations l ON l.id = t.location_id
     WHERE t.org_id = $1 AND t.type = $2 AND t.value = $3
       AND t.deleted_at IS NULL
     LIMIT 1
`, orgID, tagType, req.Value).Scan(
    &conflictAssetID, &conflictLocationID,
    &conflictAssetName, &conflictAssetExtKey,
    &conflictLocationName, &conflictLocationExtKey,
)

if err == nil {
    return &tagConflictError{
        Value:                req.Value,
        Type:                 tagType,
        AssetID:              conflictAssetID,
        AssetName:            conflictAssetName,
        AssetExternalKey:     conflictAssetExtKey,
        LocationID:           conflictLocationID,
        LocationName:         conflictLocationName,
        LocationExternalKey:  conflictLocationExtKey,
    }
}
if !errors.Is(err, pgx.ErrNoRows) {
    return err
}

// INSERT as before — the partial unique index is the belt to this suspenders
```

**Handler-level translation** (`handlers/assets/assets.go:953-963` and
mirror in `handlers/locations/locations.go`): the existing `strings.Contains(err.Error(), "already exist")`
path stays as a final safety net for the DB-constraint race, but the
typed `*tagConflictError` becomes the primary path. New 409 body shape:

```json
{
  "error": {
    "code": "tag_already_attached",
    "detail": "tag rfid:E280...001 is already attached to location \"Conference Room A\" (id 42); remove it from the conflicting entity before attaching here",
    "conflict": {
      "entity_type": "location",
      "entity_id": 42,
      "entity_name": "Conference Room A",
      "entity_external_key": "LOC-CONF-A"
    }
  }
}
```

The `conflict` object is load-bearing: frontend uses it to render the
inline error without a second lookup.

**Audit of adjacent insert sites** (per "audit adjacent surfaces"
norm): grep for every place that writes to `trakrf.tags` outside this
handler. Known sites from exploration:

- `migrations/000035`, `000036`, `000037` `.up.sql` — historical data
  backfill functions. Don't run again post-migration; constraint catches
  them if they do.
- Scan-event ingestion (MQTT and handheld submit) — verify these don't
  auto-create tag rows. If they do, they need the same pre-check or
  they'll hit the DB constraint with an unfriendly error.

The audit is part of the ticket, not a follow-up.

### Phase 3 — Frontend UX changes

**Affected files:** `AssetForm.tsx`, `LocationForm.tsx`,
`AssetFormModal.tsx`, `LocationFormModal.tsx`, `TagInputRow.tsx`.

**Behavior:**

1. **Remove the "Reassign" modal entirely.** `ConfirmModal` invocation,
   `handleConfirmReassign`, the `confirmModal` state — gone. The current
   "Tag Already Assigned" flow misled users into thinking they were
   moving a tag when they were duplicating it.

2. **Per-row inline error state.** `TagInputRow` already takes an
   `error` prop and renders a red border + message below. Add a new
   prop:
   ```ts
   conflict?: {
     entityType: 'asset' | 'location';
     name: string;
     externalKey: string;
   };
   ```
   When set, the row renders red-bordered with text:
   `⚠ Tag already attached to location "Conference Room A" (LOC-CONF-A).
    Remove it from there before attaching here.`

3. **Conflict detection — when to fire `lookupApi.byTag`:**
   - **On barcode scan** — as today (already wired in `handleBarcodeScan`).
   - **On typed input blur** — new.
   - **On submit** — belt-and-suspenders. The form's submit handler
     walks `tagInputs`, calls `lookupApi.byTags` (batch endpoint), and
     blocks submit if any new row collides.

4. **Save button disabled while any row has a conflict.** Same disabled
   state as existing-field validation errors.

5. **Catch 409 on `addTag` POST.** Even with the pre-checks, a
   concurrent insert from another session can race. The modal
   `handleSubmit` already catches `err.response?.status === 409`; extend
   it to parse the `conflict` field, stamp the inline error onto the
   offending row, and re-open the form rather than closing on the
   partial success of the PATCH.

**Subtle distinction:** if the user types a value that's already on the
**same entity** they're editing (a duplicate within the form), that's a
different error — "this tag is already in the list" — which the local
dedup check at `AssetForm.tsx:155` / `:185` already handles. Don't
conflate the two error messages.

## Test plan

**Backend (Go):**

- `storage/tags_test.go` new cases:
  - `TestAddTagToAsset_CrossAssetConflict` — insert X on A, retry on B → `*tagConflictError`, asset side.
  - `TestAddTagToAsset_CrossLocationConflict` — insert X on location L, retry on asset B → conflict naming location L.
  - `TestAddTagToLocation_CrossEntityConflict` — mirror for location handler.
  - `TestAddTagToAsset_SoftDeletedRowNotBlocking` — insert + soft-delete X on A, then insert X on B → success.
  - `TestAddTagToAsset_RaceLoses` — concurrent inserts: exactly one wins, the other gets `*tagConflictError` (verifies DB-constraint suspender).
- `handlers/assets/tag_conflict_integration_test.go` (new file, mirror for locations): POST with colliding value → 409, `code=tag_already_attached`, conflict body shape contract.

**Migration:**

- New migration test in the existing pattern: up + down + idempotency.
- Specific case: seed a duplicate, run up → expect failure with the "resolve with..." hint.

**Frontend (Vitest):**

- `TagInputRow.test.tsx` adds 3 tests:
  - Renders `conflict` prop as red-bordered error with entity name and external key.
  - `conflict` overrides `error` styling (pick precedence in test).
  - `readOnly` + `conflict` together — conflict styling wins on the readonly span.
- `AssetForm.test.tsx` / `LocationForm.test.tsx` and the two modal tests are currently excluded from vitest config; unit coverage at the `TagInputRow` level is sufficient. Re-enabling is out of scope.

**E2E (Playwright, preview):**

- One new scenario in `tests/e2e/`: create asset A with tag X → navigate to asset B's create form → scan/type X → inline error, Save disabled → delete X from A → save A → return to B → retype X → save → success.
- Run against preview after merge per the e2e batching norm.

## Migration / rollout plan

1. Land Phase 1 (schema migration) standalone in a PR. Preview gets
   hand-cleaned via the runbook UPDATE before apply. Production is empty
   so the migration applies clean.
2. Land Phase 2 (backend service) in a separate PR. Without the
   migration, the pre-check is best-effort but works; with it, it's
   bulletproof.
3. Land Phase 3 (frontend) last. Without it, the user sees a generic
   409 toast; with it, they get the inline error and resolution
   guidance.

If the work fits in one PR comfortably, fine — the phasing is for
review legibility, not for separate ship cadence.

## Open risks

- **Scan-event ingestion path.** The audit (Phase 2) may surface that
  ingestion auto-creates tag rows on first sight. If so, the constraint
  will start rejecting those writes and the ingestion path needs the
  same pre-check or a different idempotency story. Out of scope for
  this ticket as a code change, but may surface as a follow-up.
- **Concurrent insert race.** The transactional pre-check narrows the
  race but doesn't eliminate it under `READ COMMITTED`. The DB partial
  index is the final arbiter; the handler must translate the unique-
  violation `SQLSTATE 23505` back into `tag_already_attached` so the
  user-facing 409 is consistent regardless of which layer caught it.
