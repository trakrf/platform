# TRA-539 + TRA-540 — Combined API spec cleanup

**Tickets:** [TRA-539](https://linear.app/trakrf/issue/TRA-539) (OpenAPI hygiene) + [TRA-540](https://linear.app/trakrf/issue/TRA-540) (vocabulary cleanup)
**Parent epic:** [TRA-537](https://linear.app/trakrf/issue/TRA-537) (BB12 findings sweep)
**Branch:** `fix/tra-539-540-api-spec-cleanup`
**Worktree:** `.worktrees/tra-539-540-api-spec-cleanup`
**Date:** 2026-04-28

## Why combined

The §2.x hygiene fixes from TRA-539 and the §3.x rename catalog from TRA-540 share three field-level collisions:

| TRA-539 finding | TRA-540 rename | Same field |
| --- | --- | --- |
| §2.5 add closed enum to `PublicAssetView.type` | §3.1 rename `type` → `asset_type` | `asset.PublicAssetView.type` |
| §2.4 type tag-add response as `shared.TagIdentifier` | §3.5 rename `id` → `surrogate_id` on tag | tag-add 201 body |
| §2.2 mark `location.PublicLocationView.parent` omit-when-unset | §3.4 rename `parent` → `parent_identifier` (then §3.6 → `parent_location_identifier`) | `location.PublicLocationView.parent` |

Splitting into two PRs would mean touching each of these fields twice with an awkward intermediate state. One coordinated breaking-change PR matches the rationale TRA-540 already articulates: NADA has no API plans, TeamCentral has not started codegen, and the window closes the moment the first external integrator commits. One round of generated-client churn instead of N.

## Scope

### In scope
- Backend Go schema + handler annotation changes for every rename and hygiene fix
- Regenerated `backend/internal/handlers/swaggerspec/openapi.public.{yaml,json}` and synced `docs/api/openapi.public.{yaml,json}` (the two files diverge at branch creation, ~84KB vs ~90KB; regen brings them into sync)
- Frontend sweep across `frontend/src/lib/api/*.ts`, `frontend/src/types/*.ts`, components consuming these names, and test fixtures — manual since there is no generated client
- Top-level `security: [{ APIKey: [] }]` declared on the OpenAPI document (TRA-539 §2.6)

### Out of scope (explicitly deferred)
- Customer-facing docs in `/home/mike/trakrf-docs` (`docs/api/resource-identifiers`, `docs/getting-started/api`, examples) — separate trakrf-docs follow-on PR after this lands, per the "ship docs behind backend reality" discipline
- TRA-541 (HTTP envelope consistency 405/415, multipart) — sibling subtask, separate ticket
- TRA-538 (error envelope title/detail) — already in flight on PR #244
- TRA-543 (post-launch polish, e.g. 2.7) — backlog
- User-defined asset type classifications — YAGNI, no customer asking
- Database column renames — `surrogate_id` exists at the API layer only; columns stay `id`
- Deprecation/dual-name shims — clean break, no external integrator yet

## Vocabulary decisions

### Default asset type name
`asset.type = "asset"` is tautological in generated SDKs (`Asset(asset_type=AssetType.ASSET)`). Renamed to `item`.

Rationale: three production/historical use cases — asset/equipment tracking, inventory tracking, person tracking. `item` is the most semantically generic of the candidates (`equipment`, `tool`, `general`, `item`) and pairs cleanly with `person` and `inventory` as orthogonal tracking modes.

Final enum on `asset.PublicAssetView.asset_type`: `[item, person, inventory]`.

### Location reference field naming
Every Location reference standardizes on natural-key string identifier: `*_location_identifier`. No integer surrogates in response shapes.

| Old | New |
| --- | --- |
| `report.PublicAssetHistoryItem.location` | `location_identifier` |
| `report.PublicCurrentLocationItem.location` | `location_identifier` |
| `asset.PublicAssetView.current_location` | `current_location_identifier` |
| `inventory.SaveRequest.location_identifier` | unchanged (already correct) |
| `storage.SaveInventoryResult.location_id` (int surrogate) | `location_identifier` (natural key) |
| `location.CreateLocationWithTagsRequest.parent_identifier` | `parent_location_identifier` |
| `location.UpdateLocationRequest.parent_identifier` | `parent_location_identifier` |
| `location.PublicLocationView.parent` | `parent_location_identifier` |

The `parent_identifier` → `parent_location_identifier` step extends TRA-540 §3.4 to match the full §3.6 standardization. Catalog addendum recorded here.

### Surrogate ID convention
Every integer surrogate in a public response is named `surrogate_id`. `id` no longer appears on public response surfaces. `jti` is preserved on api-key shapes — it is a stable UUID, not a surrogate.

| Schema | Old | New |
| --- | --- | --- |
| `apikey.APIKeyCreateResponse` | `id` | `surrogate_id` |
| `apikey.APIKeyListItem` | `id` | `surrogate_id` |
| `orgs.OrgMeView` | `id` | `surrogate_id` |
| `shared.TagIdentifier` | `id` | `surrogate_id` |
| `asset.PublicAssetView.surrogate_id` | unchanged | unchanged |
| `location.PublicLocationView.surrogate_id` | unchanged | unchanged |

### Tag type field
`shared.TagIdentifier.type` → `tag_type`. Enum unchanged: `[rfid, ble, barcode]`. Eliminates the `type=rfid` / `type=item` cross-resource collision that TRA-540 §3.1 calls out.

### Path parameter rename
`DELETE /api/v1/assets/{identifier}/tags/{tagId}` path parameter renamed to `{tagSurrogateId}` (TRA-540 §2.8) to match the response field name.

### Route rename
`POST /api/v1/inventory/save` → `POST /api/v1/scans` (TRA-540 §3.2). Aligns with the `scans:write` scope name that already gates it. Eliminates the `models.Inventory` / `inventoryClient.save()` SDK ambiguity that conflated the consumable-goods asset type with the scan-event ingest endpoint.

## Spec hygiene decisions

### Omit-when-unset semantics (TRA-539 §2.1, §2.2, §2.3)
The service has been omitting these optional fields all along. `docs/api/date-fields` already documents the omit-when-empty contract for `valid_to`. The spec is the outlier.

**Decision: change the spec to match the service.** Mark fields as not-required and remove `nullable: true`. Switching to always-emit-null is a behavior change that would break internal frontend consumers that follow the documented "test for key presence" pattern.

Fields affected:
- `apikey.APIKeyCreateResponse.expires_at` — omit-when-unset (§2.1)
- `apikey.APIKeyListItem.expires_at` — omit-when-unset (§2.1)
- `asset.PublicAssetView.description` — omit-when-unset (§2.2)
- `asset.PublicAssetView.valid_to` — omit-when-unset (§2.2)
- `location.PublicLocationView.valid_to` — omit-when-unset (§2.2)
- `location.PublicLocationView.parent_location_identifier` — omit-when-unset (§2.2 + §3.4 + §3.6)
- `report.PublicCurrentLocationItem.asset_deleted_at` — omit-when-unset (§2.3)

### Tag-add 201 response shape (TRA-539 §2.4)
`POST /api/v1/assets/{identifier}/tags` and `POST /api/v1/locations/{identifier}/tags` declare 201 response as `additionalProperties: true, type: object`. Service returns `{"data": shared.TagIdentifier}`. Spec gets typed to match — generated clients hand integrators a real `Tag` instead of `Map<String, Object>`.

### Asset type response enum (TRA-539 §2.5)
Apply closed enum `[item, person, inventory]` to the response side of `asset.PublicAssetView.asset_type` (now renamed per §3.1). Was `type: string` open. Generated clients can now round-trip a fetched value back into a write without a cast.

### Top-level security block (TRA-539 §2.6)
Add at document level:
```yaml
security:
  - APIKey: []
```
Currently `components.securitySchemes` defines `APIKey` and `BearerAuth` but no operation declares one and no top-level block exists. Without it, generated clients ship every call unauthenticated unless the developer manually attaches the header. Three lines of YAML — cheapest fix in the epic.

Internal endpoints that should not require `APIKey` (none expected on the public spec, but verify during build) get an explicit `security: []` override.

## Implementation plan

### Source of truth
Backend Go struct and handler annotations are the source of truth. The OpenAPI yaml is generated via `just backend api-spec`. The yaml is committed but not hand-edited.

### Frontend consumption
Hand-written modules in `frontend/src/lib/api/*.ts` (no codegen). Each rename is a manual sweep — TS compiler enumerates consumer breakage after type renames.

### Per-resource commit plan (Approach B)
Six backend commits, each bundling the §2.x and §3.x changes for one resource so colliding fields are touched once. Then a spec-regen commit and a frontend-sweep commit.

#### Commit 1 — `asset` resource
- Rename `asset.PublicAssetView.type` → `asset_type` (§3.1)
- Apply closed enum `[item, person, inventory]` to both request and response (§2.5, §3.3)
- Rename `asset.PublicAssetView.current_location` → `current_location_identifier` (§3.6)
- Mark `description` and `valid_to` as not-required, remove `nullable: true` (§2.2)
- `surrogate_id` keeps name (already correct)

#### Commit 2 — `location` resource
- Rename `location.PublicLocationView.parent` → `parent_location_identifier` (§3.4 + §3.6 merged)
- Rename request-side `parent_identifier` → `parent_location_identifier` on `CreateLocationWithTagsRequest` and `UpdateLocationRequest`
- Mark `valid_to` and `parent_location_identifier` as not-required (§2.2)
- Rename `storage.SaveInventoryResult.location_id` → `location_identifier` (§3.6, natural key)
- `surrogate_id` keeps name

#### Commit 3 — `tag` resource and tag-add endpoints
- Rename `shared.TagIdentifier.id` → `surrogate_id` (§3.5)
- Rename `shared.TagIdentifier.type` → `tag_type` (§3.1) with enum `[rfid, ble, barcode]` preserved
- Type 201 responses on `POST /assets/{identifier}/tags` and `POST /locations/{identifier}/tags` as `{"data": shared.TagIdentifier}` (§2.4)
- Rename path param `DELETE …/tags/{tagId}` → `{tagSurrogateId}` (§2.8)

#### Commit 4 — `apikey` resource
- Rename `apikey.APIKeyCreateResponse.id` → `surrogate_id` (§3.5)
- Rename `apikey.APIKeyListItem.id` → `surrogate_id` (§3.5)
- Mark `expires_at` as not-required on both shapes (§2.1)
- `jti` left alone

#### Commit 5 — `report` resource
- Rename `report.PublicAssetHistoryItem.location` → `location_identifier` (§3.6)
- Rename `report.PublicCurrentLocationItem.location` → `location_identifier` (§3.6)
- Mark `report.PublicCurrentLocationItem.asset_deleted_at` as not-required (§2.3)

#### Commit 6 — `scans` route + `orgs` + security block
- Move `POST /api/v1/inventory/save` → `POST /api/v1/scans` (§3.2)
- Rename `orgs.OrgMeView.id` → `surrogate_id` (§3.5)
- Add top-level `security: [{ APIKey: [] }]` at the document level (§2.6)
- `inventory.SaveRequest.location_identifier` keeps name

#### Commit 7 — Spec regen
- Run `just backend api-spec`
- Commit `backend/internal/handlers/swaggerspec/openapi.public.{yaml,json}`
- Sync `docs/api/openapi.public.{yaml,json}` so the two files match
- Eyeball-diff against the rename catalog as a sanity check

#### Commit 8 — Frontend sweep
- Rename TS types in `frontend/src/types/*.ts` and `frontend/src/lib/api/*.ts`; let TS compiler enumerate consumer breakage
- Walk failures, update each call site
- Update mocks/fixtures in `*.test.ts` and `*.test.tsx`
- `inventory.ts` likely renames to `scans.ts` for consistency with the route
- Run `just frontend lint && just frontend test` until clean
- Browser smoke test against local dev server: inventory/scan + asset CRUD + apikey create-list + location create-with-tags

### Per-rename enumeration discipline
For each renamed identifier, grep the entire platform repo (Go + TS + tests + comments + fixtures) for every occurrence of the old name before claiming the rename complete. Per the "rename sweep enumeration" feedback memory, surface every renamed object explicitly — not just primary type names.

## Verification

### Per-commit
- `just backend test` after each backend commit
- `just frontend lint && just frontend test` after the frontend commit
- `just validate` (combined) before pushing the branch
- Local browser smoke test before opening the PR

### Post-PR (against `app.preview.trakrf.id`)
- BB-style re-run of §2.x and §3.x reproduction calls from BB12 FINDINGS.md — acceptance bar for both tickets
- Generate a throwaway client from the regenerated spec (e.g. `openapi-generator-cli generate -i …`) and compile it cleanly against the live preview service for the BB12 reproduction calls (TRA-539 acceptance)
- No per-ticket Playwright run — API surface work, batched verification per the "API only, batched" discipline

## Risk and rollback

### Risk surface
- **Frontend regression that compiles but misbehaves.** TS rename catches name changes but not field-presence checks (`if (response.parent)`). Mitigation: dev-server smoke test of inventory/scan, asset CRUD, location CRUD, apikey lifecycle.
- **Spec regen drift.** The two yaml files already differ at branch creation. Regen must bring both into sync. Mitigation: dedicated regen commit, confirm the two files are byte-identical post-regen.
- **Half-renamed surface escapes the sweep.** Mitigation: per-renamed-object grep across the whole platform repo before each commit.
- **External integrator surprise.** Confirmed none. NADA uses the UI; TeamCentral has not started codegen.

### Rollback
- Single PR = single revert. Per-resource commits give granular revert before merge; post-merge, the whole PR is the rollback unit.
- Breaking-change nature means there is no graceful partial rollback. If staging breaks, revert the merge commit.

### Decision-drift discipline
Any rename not in this catalog discovered mid-implementation gets recorded as an addendum on TRA-540's body before being shipped. Same for any new spec-vs-service mismatch surfaced by the BB-style re-run — addendum on TRA-539. No silent additions.

## Acceptance

Combined acceptance from both tickets:
- [ ] All §3.1–3.6 + §2.8 renames applied across spec, Go handlers, response serializers, request validators
- [ ] All §2.1–2.6 hygiene fixes applied
- [ ] Internal frontend code consuming the new names; TS lint and tests passing
- [ ] OpenAPI spec regenerated and committed; both yaml copies in sync
- [ ] All endpoints in BB12 §4 walk re-tested against the new spec — every CRUD lifecycle verified against preview
- [ ] BB-style re-run produces zero §2.x findings, zero §3.x findings, zero §2.8 finding
- [ ] Generated client compiles cleanly against live preview for BB12 reproduction calls
- [ ] Database column names left alone — surrogate_id at API layer only

## References

- BB12 [FINDINGS.md](https://uploads.linear.app/...) — eval doc evaluated 2026-04-27 against `app.preview.trakrf.id`
- BB.md — eval methodology
- `/home/mike/platform/docs/api/openapi.public.yaml` — spec served via redocusaurus
- `/home/mike/platform/backend/internal/handlers/swaggerspec/openapi.public.yaml` — backend-served copy
- `/home/mike/platform/docs/api/date-fields` (in trakrf-docs) — already documents the omit-when-empty contract
- TRA-210 — Public REST API epic (Done) — context for the v1 surface being evaluated
