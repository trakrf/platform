# Tag Conflict UX — Design

**Date:** 2026-05-22
**Ticket:** TRA-806
**Status:** Approved, pending implementation plan
**Supersedes:** `2026-05-19-tag-uniqueness-cross-entity-design.md` (PR #383 — rebaselined; see "What changed" below)
**Related:** TRA-482 (the migration that already closed the integrity hole), TRA-799 (asset location → scan-derived fact data), TRA-803 (adjacent tag-submit bug, tracked separately)

## What changed from the 2026-05-19 spec

The earlier spec was written against stale knowledge and is rebaselined here:

- **Phase 1 (schema migration) is dropped.** The partial unique index it
  proposed already exists — `tags_org_id_type_value_unique` on
  `tags(org_id, type, value) WHERE deleted_at IS NULL`, added by migration
  `000032_unique_identifiers_partial` (TRA-482, 2026-04-24) and renamed onto
  `tags` by `000033` (TRA-524). The per-row `tag_target` CHECK already
  enforces asset-XOR-location.
- **The "Problem" framing is corrected.** The earlier spec claimed the schema
  silently allows duplicates and that `parseTagError`'s 409 path is
  unreachable. Both are false: the index fires on a duplicate attach,
  `parseTagError` matches the constraint, and the handler already returns
  409. The data-integrity hole is already closed.
- **The work is now purely UX**, restructured into two workstreams (Backend /
  Frontend) instead of three phases.

## Problem

A single physical RFID/BLE tag (EPC) attaches to only one entity at a time —
one asset XOR one location. The database already enforces this. The defect is
the experience around a collision:

1. **Backend — the 409 is not actionable.** A duplicate attach returns 409,
   but the detail is `"tag rfid:XXX already exists"`. It does not name the
   asset or location the tag is already on, so the user cannot act on it.

2. **Frontend — asymmetric and misleading.**
   - **Asset form** surfaces the save-time 409 to the user.
   - **Location form** swallows it — the save **silently fails**. The
     backend (`doAddLocationTag`, `locations.go`) returns the same 409 as
     the asset path; the location create/update **caller** drops it instead
     of feeding the form's `error` prop.
   - Both forms show a **"Tag Already Assigned" modal** whose **"Reassign"**
     action (`handleConfirmReassign`) just appends the value to the form's
     local tag list. It does not detach the tag from the old entity. The
     user believes they moved a tag; they created a collision that then
     409s (asset) or fails silently (location) on save.
   - Conflict detection only fires on **barcode scan**, never on typed
     input — a typed EPC collision is invisible until save.

TRA-799 made an asset's location *scan-derived fact data*. The tag → entity
resolution is now the load-bearing join behind an asset's location, so a
clean attach experience matters more, not less.

## Goals

- Make the 409 actionable: name the conflicting entity in the message.
- Bring the location form to parity with the asset form — surface the 409.
- Detect a collision **before save**, on typed input, on both forms.
- Replace the misleading "Reassign" modal with honest inline error state.

## Non-Goals

- **Schema changes.** The uniqueness constraint already exists.
- **An error-envelope schema change.** The 409 carries a richer `detail`
  string within the existing `modelerrors.ErrorResponse`; no structured
  `conflict` object, no `openapi.public.yaml` design pass. The proactive
  onblur check already has structured entity data from `lookupApi.byTag`,
  so the save-time 409 only needs to be human-readable.
- **An atomic reassign endpoint.** The workflow stays explicit: remove the
  tag from the old entity, then add it to the new one.
- **TRA-803** (asset edit: a tag-only change does not persist on Update).
  Adjacent — same `AssetForm` submit path — but tracked separately.
- **Scan-event ingestion EPC → entity resolution.** Unchanged.

## Design

Two workstreams. They may land in one PR or two; the split is for review
legibility, not ship cadence.

### Backend — actionable 409 detail

Both `doAddAssetTag` (`handlers/assets/assets.go`) and `doAddLocationTag`
(`handlers/locations/locations.go`) already translate the storage error into
a 409 via the `strings.Contains(err.Error(), "already exist")` branch. No
asymmetry to fix here — the win is the message.

Today, `storage.parseTagError` maps the unique-violation
(`ConstraintName == "tags_org_id_type_value_unique"`, SQLSTATE 23505) to
`"tag <type>:<value> already exists"`. `parseTagError` has no transaction
handle, so it cannot look up the conflicting entity itself.

The enrichment goes in `AddTagToAsset` / `AddTagToLocation`
(`storage/tags.go`), which hold the `WithOrgTx` handle. On a unique-violation
error from the INSERT, run a follow-up lookup within the same org scope:

```sql
SELECT t.asset_id, t.location_id,
       a.name, a.external_key,
       l.name, l.external_key
  FROM trakrf.tags t
  LEFT JOIN trakrf.assets    a ON a.id = t.asset_id
  LEFT JOIN trakrf.locations l ON l.id = t.location_id
 WHERE t.org_id = $1 AND t.type = $2 AND t.value = $3
   AND t.deleted_at IS NULL
 LIMIT 1
```

Build the enriched error:

> `tag rfid:E280…001 is already attached to location "Conference Room A" (LOC-CONF-A); remove it there before attaching here`

The happy path is unchanged — a single INSERT. The lookup runs only on the
rare collision. The handler's existing `already exist` → 409 branch passes
the enriched string straight through as the 409 `detail`.

If the lookup itself returns no row (the conflicting row was soft-deleted
between the INSERT failing and the lookup — a narrow race), fall back to the
current generic `"tag <type>:<value> already exists"` message.

### Frontend — three changes, applied symmetrically to both forms

Affected: `AssetForm.tsx`, `LocationForm.tsx`, `TagInputRow.tsx`,
`vitest.config.ts`, and the location create/update caller.

**1. Location 409 parity.**
Trace the location create/update path that currently swallows the 409 and
surface it into the form's `error` prop, matching how the asset path feeds
`<ErrorBanner error={error} />` (`AssetForm.tsx:278`). After this, a
save-time collision on the location form shows the same banner the asset
form already shows.

**2. Onblur conflict check (both forms).**
Each tag row already has an `onBlur` handler (`AssetForm.tsx:478`,
`LocationForm.tsx:557`) that only calls `setFocusedTagIndex(null)`. Extend
it: on blur of a non-empty tag value, call `lookupApi.byTag('rfid', value)`.
- A hit attached to a **different** entity → stamp an inline conflict error
  on that row (see change 3).
- `404` (tag not found anywhere) → clear any conflict state on the row.
- Other errors → leave the row unmarked (best-effort; the save-time 409 is
  the backstop).

Skip the check when the value duplicates another row in the **same** form —
that is the existing local-dedup case ("this tag is already in the list"),
a different message; do not conflate the two.

**3. Remove the "Reassign" modal; inline conflict error instead.**
Delete from both forms: the `confirmModal` state, `handleConfirmReassign`,
and the `ConfirmModal` JSX block (`AssetForm.tsx:518-`,
`LocationForm.tsx:597-`). The barcode-scan path that currently opens the
modal sets the same inline conflict state instead.

`TagInputRow` gains a `conflict` prop:

```ts
conflict?: {
  entityType: 'asset' | 'location';
  name: string;
  externalKey: string;
};
```

When set, the row renders red-bordered with a message naming the conflicting
entity, e.g.:

> ⚠ Tag already attached to location "Conference Room A" (LOC-CONF-A).
> Remove it from there before attaching here.

The Save button is disabled while any row carries an unresolved conflict —
the same disabled treatment as existing field-validation errors.

**4. Save-time 409 catch (race backstop).**
Even with the onblur check, a concurrent attach from another session can
race. Keep catching the 409 on save; show the backend's enriched `detail`
string in the form error banner. With change 1, this now works on the
location form too.

## Test plan

**Backend (Go):**

- `storage/tags_test.go`:
  - `TestAddTagToAsset_CrossAssetConflict` — value X on asset A, retry on
    asset B → enriched error names A.
  - `TestAddTagToAsset_CrossLocationConflict` — value X on location L,
    retry on asset B → enriched error names location L.
  - `TestAddTagToLocation_CrossEntityConflict` — mirror for the location
    handler.
  - `TestAddTagToAsset_SoftDeletedRowNotBlocking` — attach + soft-delete X
    on A, then attach X on B → success.
- Handler integration test (`handlers/assets/` and `handlers/locations/`):
  POST a colliding tag → 409, `detail` names the conflicting entity.

**Frontend (Vitest):**

- `TagInputRow.test.tsx`: renders the `conflict` prop as a red-bordered
  error with entity name and external key; `conflict` precedence over a
  plain `error`.
- `AssetForm` / `LocationForm`: onblur fires `lookupApi.byTag` and stamps a
  conflict on a cross-entity hit; clears on `404`; Save disabled while a row
  is conflicted; the location form surfaces a save-time 409 into the error
  banner.
- **Re-enable the excluded form vitest files.** `AssetForm.test.tsx` and
  `AssetFormModal.test.tsx` sit in the `vitest.config.ts` exclude list under
  the `TRA-192: Tests with incomplete store mocks` block. Remove both
  entries, repair the store mocks so the suites pass, and add the
  conflict/onblur cases above to `AssetForm.test.tsx`. `LocationForm.test.tsx`
  already runs (not excluded) — add the same cases there.

**E2E (Playwright, preview):**

- One scenario: create asset A with tag X → open asset B's form → type X,
  blur → inline conflict error, Save disabled → detach X from A → return to
  B, retype X → save succeeds. Run against preview after merge, per the e2e
  batching norm.

## Rollout

1. Backend 409 enrichment and the frontend changes are independent and may
   land in either order or together. Without the backend change, the
   save-time banner shows the generic message; without the frontend change,
   the enriched 409 still surfaces (asset) or is still swallowed (location).
2. No migration, no preview data cleanup, no production gate.

## Open risks

- **Locating the swallowed location 409.** The exact caller that drops the
  location save error is not yet pinned (parent component or store action).
  The implementation plan resolves this; it does not change the design.
- **Onblur lookup latency.** `lookupApi.byTag` is a network round-trip on
  every tag-field blur. Acceptable for the form's interaction rate; the
  save-time 409 remains the correctness backstop if a check is slow or
  skipped.
- **Store-mock repair scope.** `AssetForm.test.tsx` and
  `AssetFormModal.test.tsx` were excluded (TRA-192) for incomplete store
  mocks — a pre-existing gap, not caused by this feature. Re-enabling them
  means repairing those mocks, and the size of that repair is unknown until
  the suites are run. If it balloons, the mock repair can land as its own
  commit within the PR.
