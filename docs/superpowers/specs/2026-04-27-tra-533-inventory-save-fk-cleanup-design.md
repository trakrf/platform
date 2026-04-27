# TRA-533 — `POST /api/v1/inventory/save` cleanup: drop surrogate aliases, lock in regression tests

## Problem

`POST /api/v1/inventory/save` 500s on every well-formed payload were surfaced by BB11 (TRA-532 finding S3) hours after TRA-524 (identifier→tag rename) hit preview. Root cause: a missed FK column rename (`asset_scans.identifier_scan_id` → `tag_scan_id`) inside an embedded SQL string in `backend/internal/storage/inventory.go`. That miss was fixed in commit c678465 during the TRA-524 testing pass.

What TRA-533 ships now, on top of that already-shipped fix:

1. **Confirm no embedded-SQL misses remain** (AC1 — already complete; documented here).
2. **Remove the undocumented `_id`/`_ids` request aliases** on `/inventory/save` so the public surface has one canonical shape (AC2, absorbing TRA-532 finding F10).
3. **Add the two missing regression-test cases** so the next embedded-SQL miss on this endpoint fails CI before it hits preview (AC3).

## AC1 verification (already shipped, documented for the record)

A grep over the Go backend for old-name references — including `identifier_scan_id`, `trakrf.identifiers`, `trakrf.identifier_scans`, raw `FROM/JOIN/INTO identifiers|identifier_scans`, and the migration-renamed PL/pgSQL function/trigger/sequence/index/constraint/policy names — returns zero hits outside `backend/migrations/000033_rename_identifier_to_tag.down.sql` (which legitimately references the old names because it reverses the rename).

The `identifier` natural-key column on entity tables (assets, locations, organizations, scan_devices, scan_points) is the unchanged TRA-524 convention. References to `assets.identifier`, `locations.identifier`, etc. are correct and stay.

c678465 covered both real misses (`backend/internal/storage/inventory.go:93` and the `backend/internal/handlers/reports/public_integration_test.go` fixture). No further code changes for AC1.

The regression test added under AC3 is the lock-in for AC1 — if a future column rename misses the inventory/save persist path, the test fails.

## Approach

Two coordinated cuts (backend + frontend, single PR) plus added test coverage.

**Cut #1 — Backend collapses to a single request shape.** `SaveRequest` drops `LocationID` and `AssetIDs`. Required-ness moves into struct tags. Cross-field branches that existed only to support the dual shape delete with the fields they guarded.

**Cut #2 — Frontend follows.** `SaveInventoryRequest` type renames its fields to natural keys; `InventoryScreen.tsx` passes `resolvedLocation.identifier` and asset identifier strings instead of surrogate IDs. The 403 warning log in `useInventorySave.ts` updates field names.

**Lock-in tests.** Two new integration cases (multi-asset → 201, empty `asset_identifiers` → 400) and one explicit alias-rejection canary (`{location_id, asset_ids}` → 400). Existing numeric-ID happy-path tests rework to use identifiers, preserving auth coverage on the surviving shape.

## Backend changes

### `backend/internal/handlers/inventory/save.go`

`SaveRequest` collapses from four fields to two:

```go
type SaveRequest struct {
    LocationIdentifier *string  `json:"location_identifier" validate:"required,min=1,max=255" example:"WH-01"`
    AssetIdentifiers   []string `json:"asset_identifiers" validate:"required,min=1,dive,min=1,max=255" example:"ASSET-0001"`
}
```

Notes:
- `validate:"required"` (not `omitempty`) — moves required-ness into struct tags so `validate.Struct` produces field errors directly.
- `LocationIdentifier` stays a pointer to keep `null`-vs-missing distinguishable (existing pattern).
- `swaggerignore:"true"` is gone with the fields it qualified.

The `Save` handler body simplifies to: decode → validate → resolve location → resolve assets → persist. Specifically:

- Cross-field blocks at lines 104–143 (the `location_identifier or location_id is required`, `asset_identifiers or asset_ids is required`, "specify either, not both") **delete entirely**.
- The `location_identifier and location_id disagree` branch at lines 164–174 **deletes**.
- Resolution blocks for location and assets stay, but unconditionally — `if hasLocIdent` / `if hasAssetIdents` gates remove since validation guarantees both are populated.

### Decode strictness for legacy clients

`httputil.DecodeJSON` strictness will be checked during implementation. Two outcomes for a payload like `{location_id: 123, asset_ids: [...]}` (no identifier fields):

- **Strict (`DisallowUnknownFields`)**: rejected at decode with "unknown field location_id" — crisp.
- **Loose**: decodes to empty struct, fails validation with `"location_identifier is required"` — acceptable but less crisp.

If the project's pattern for public endpoints is strict, this handler may switch — implementation-time decision, not a spec one. Either outcome is correct user-facing behavior.

### `backend/internal/handlers/inventory/save_test.go`

Mock-storage unit tests. Drop alias-shape branches; keep coverage of:

- Missing `location_identifier` → 400 with field error
- Missing `asset_identifiers` → 400 with field error
- Validator field errors (length, dive) → 400
- Location-not-found → 400
- Asset-not-found(s) → 400 with per-missing field errors
- Storage access-denied → 403
- Mock OK → 201

Net file shrinks.

### `backend/internal/handlers/inventory/public_write_integration_test.go`

Real-DB integration tests. Changes:

- `TestInventorySave_APIKey_HappyPath` (line 88): rework to use identifiers — becomes AC3's **single-asset → 201** case.
- `TestInventorySave_SessionAuth_HappyPath` (line 147): rework to use identifiers (preserves session-auth coverage).
- `TestInventorySave_APIKey_Identifiers_HappyPath` (line 225): redundant with the reworked happy path; **delete** (or merge if useful structure remains).
- New: **multi-asset → 201** (AC3).
- New: **empty `asset_identifiers` → 400** (AC3).
- New: **legacy-shape rejection** — request with `{location_id, asset_ids}` (no identifier fields) → 400 with `location_identifier` field error. Explicit canary for AC2.
- Existing 403 cross-org and 403 wrong-scope: keep unchanged.
- Existing 400 location-not-found and 400 asset-not-found: keep, adjust to identifier shape.
- Existing cross-field disagree test: **delete** (branch is gone).

### `backend/internal/storage/inventory.go`

No changes. Already correct post-c678465.

### Swagger / OpenAPI regen

After struct change, run the project's swagger regen recipe and inspect the diff. Expected:

- `location_id` and `asset_ids` properties drop from the internal-swagger request schema (they were `swaggerignore:"true"` so the public spec is unaffected).
- `location_identifier` and `asset_identifiers` flip from `omitempty` to required in the schema.

Nothing else should move. If anything else moves, investigate before committing.

## Frontend changes

### `frontend/src/lib/api/inventory.ts`

```ts
export interface SaveInventoryRequest {
  location_identifier: string;
  asset_identifiers: string[];
}
```

`SaveInventoryResponse` stays unchanged (response-shape `location_id` surrogate is OOS — see Out of Scope).

### `frontend/src/components/InventoryScreen.tsx:241-251`

`saveableAssets` is built from the FE tag store, where each tag entry carries both `assetId` (surrogate) and `assetIdentifier` (natural key). Tag enrichment populates both fields together (`tagStore.ts:437` and surrounding writes), so swapping the filter and map from `assetId` to `assetIdentifier` is equivalent in selection semantics.

Construction at lines 241–243 changes from:

```ts
const saveableAssets = tags
  .filter(t => t.type === 'asset' && t.assetId)
  .map(t => t.assetId!);
```

to:

```ts
const saveableAssetIdentifiers = tags
  .filter(t => t.type === 'asset' && t.assetIdentifier)
  .map(t => t.assetIdentifier!);
```

Call site at lines 248–250 changes from surrogate-IDs to natural keys:

```ts
await save({
  location_identifier: resolvedLocation.identifier,
  asset_identifiers: saveableAssetIdentifiers,
});
```

`Location.identifier` already exists on the FE `Location` type. No type/store ripple. Implementation will verify enrichment populates `assetIdentifier` whenever it populates `assetId` — if there's a code path that sets only the surrogate, that's a pre-existing bug surfaced by this change, but no current evidence suggests one.

### `frontend/src/hooks/inventory/useInventorySave.ts:54-58`

403 console warn updates from `location_id`/`asset_ids_count` to `location_identifier`/`asset_identifiers_count`. Pure log-string change.

### Test updates

- `frontend/src/hooks/inventory/useInventorySave.test.ts` — fixture data passed to `save()` updates to natural-key shape.
- `frontend/src/components/__tests__/InventoryScreen.test.tsx` — mocked `save()` call assertions update.
- `frontend/src/components/__tests__/InventoryScreen.authgate.test.tsx` — auth-gate test, likely no change; verify during implementation.

### Not changing

- `Asset` and `Location` types (they keep both `id` and `identifier`).
- Any component, store, or hook outside the inventory-save call path.
- Any UI label or component name (TRA-525 territory).

## Verification

Run in this order before claiming green.

1. `just backend test ./internal/handlers/inventory/...` — touched test files pass, including the three new cases.
2. `just backend test ./...` — full suite, catches collateral fallout.
3. `just backend lint` — vet + golangci-lint clean.
4. `just frontend test src/hooks/inventory src/components/__tests__` — touched FE tests pass.
5. `just frontend test` — full FE unit suite. TypeScript compile is the load-bearing check; the `SaveInventoryRequest` rename surfaces every miss in the call graph.
6. `just frontend lint` — clean.
7. `just validate` — full lint + test across both workspaces.
8. Swagger regen recipe — inspect diff matches expectations above.

E2E: skip locally per project convention. After the PR triggers preview deploy, run a single Playwright smoke against `https://app.preview.trakrf.id` covering the inventory scan-and-save user flow.

### Manual preview check — the BB11 repros

After preview is green, curl the three TRA-533 ticket payloads against the preview API directly:

1. `{location_identifier: "WHS-01", asset_identifiers: ["ASSET-0001"]}` → expect 201.
2. `{location_identifier: "WHS-01", asset_identifiers: ["ASSET-0001", "ASSET-0002"]}` → expect 201.
3. `{location_id: 542787020, asset_identifiers: ["ASSET-0001"]}` → expect **400** with `location_identifier` field error.

Note that #3's expected outcome inverts the original ticket's "should return 200" — the ticket pre-dates the AC2 alias-removal decision. After AC2, that exact payload *should* fail 400 with a field error pointing at `location_identifier`. The PR description will call this out so it doesn't read as a missed AC.

## Out of scope

Explicit non-goals:

- **C2 spelling proliferation.** The four-spelling analysis (`id` / `surrogate_id` / `jti` / `location_id`) from TRA-532 finding C2 stays in its own sibling-to-TRA-523 ticket. This PR fixes the inventory/save instance only.
- **Response-shape `location_id` surrogate.** `SaveInventoryResult.LocationID` (backend storage struct) and `SaveInventoryResponse.location_id` (FE type) still expose the numeric surrogate in the API response envelope. AC2 targets request-side aliases only. Response cleanup is C2 territory.
- **Broader SQL-canary harness.** No test-time canary that scans Go SQL strings against `information_schema`. No follow-up ticket. Future column renames rely on the same grep + manual sweep, with the regression test added here as the inventory/save-specific lock-in.
- **TRA-525 frontend identifier→tag work.** No UI labels, no `TagIdentifiersModal` rename, no `TagIdentifier` type rename. The FE changes here are limited to the inventory-save call-site shape.
- **Other inventory-adjacent endpoints.** Reports endpoints, tag CRUD, any future `GET /inventory/...` — out of scope. Grep already confirmed no SQL misses elsewhere.
- **Decode-strictness sweep.** Even if this handler switches to `DisallowUnknownFields` for crisper legacy-payload errors, no audit of other handlers for the same pattern.

## Sequencing and PR

Single PR with both backend and frontend changes. CLAUDE.md mandates PR-only (no local merges, no parallel worktree merges). Branch uses functional prefix per project convention (e.g., `fix/tra-533-inventory-save-fk-cleanup`).
