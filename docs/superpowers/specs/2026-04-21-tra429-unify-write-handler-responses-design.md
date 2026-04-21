# TRA-429 — Unify asset/location write-handler responses to PublicAssetView / PublicLocationView

**Linear:** [TRA-429](https://linear.app/trakrf/issue/TRA-429/unify-assetlocation-write-handler-responses-to)
**Related:** TRA-210 (public read-endpoint shapes), TRA-427 (frontend normalize band-aid), TRA-433 (contract cleanup sibling)
**Date:** 2026-04-21
**Status:** Approved — ready for implementation plan

## Problem

TRA-210 introduced `PublicAssetView` / `PublicLocationView` (drop `id` / `org_id`, expose `surrogate_id`, replace FK with natural key) and migrated read endpoints to emit them. **Write** endpoints still return the internal `AssetView` / `Asset` / `LocationView` / `Location` shapes.

Result: `POST /api/v1/assets` returns `{id, ...}` while `GET /api/v1/assets/{identifier}` returns `{surrogate_id, ...}`. Both are tagged `@Tags assets,public` — the public contract is internally inconsistent. TRA-427 was a direct consequence: the frontend cached a GET response without normalizing, producing a phantom `null`-keyed cache entry. TRA-427's fix patched the symptom with a defensive `normalizeAsset` helper; this issue removes the root cause.

## Affected endpoints

| Route | Current return shape | Target |
|-------|---------------------|--------|
| `POST /api/v1/assets` | `*asset.AssetView` | `asset.PublicAssetView` |
| `PUT /api/v1/assets/{identifier}` | `*asset.Asset` | `asset.PublicAssetView` |
| `PUT /api/v1/assets/by-id/{id}` | `*asset.Asset` (shares `doUpdateAsset`) | `asset.PublicAssetView` |
| `POST /api/v1/locations` | `*location.LocationView` | `location.PublicLocationView` |
| `PUT /api/v1/locations/{identifier}` | `*location.Location` | `location.PublicLocationView` |
| `PUT /api/v1/locations/by-id/{id}` | `*location.Location` (shares `doUpdate`) | `location.PublicLocationView` |

Unchanged: Delete (204), AddIdentifier (returns `TagIdentifier`), RemoveIdentifier (204), all LIST and GET endpoints.

## Approach

**Decisions** (chosen during brainstorming):
1. **Widen storage return types** to the rich view (`*AssetWithLocation` / `*LocationWithParent`). Handlers become one-line projections via `ToPublicAssetView` / `ToPublicLocationView`. Chosen over "post-update fetch" and "hybrid new method" because the only callers of the affected storage methods are the handlers themselves plus storage tests — blast radius is entirely in-tree.
2. **Unify create paths.** Drop the private `createAssetWithoutIdentifiers` / `createLocationWithoutIdentifiers` handler helpers. Always call `storage.CreateAssetWithIdentifiers` / `storage.CreateLocationWithIdentifiers` (both correctly handle an empty identifiers slice). One code path per resource.

## Storage changes

Four methods change signature. Pattern is identical across assets and locations.

### `storage.UpdateAsset`
- **Signature:** `(ctx, orgID, id, req) (*asset.AssetWithLocation, error)` (was `*asset.Asset`).
- **Approach:** keep the UPDATE query as-is (bare `RETURNING a.*`). After a successful update, delegate to the new `getAssetWithLocationByID` helper (see `CreateAssetWithIdentifiers` below) to fetch the LEFT-joined parent natural key + identifiers. This avoids the `UPDATE ... FROM` INNER-JOIN pitfall (Postgres `UPDATE ... FROM locations l` with `l.id = a.current_location_id` filters out rows whose FK is null) and reuses the same query shape as read endpoints. One extra round-trip, acceptable for writes.
- **Unchanged:** `(nil, nil)` on no-match; existing unique-violation / FK-violation / `current_location_id_fkey` error mapping.

### `storage.CreateAssetWithIdentifiers`
- **Signature:** `(..., req) (*asset.AssetWithLocation, error)` (was `*asset.AssetView`).
- Replace the trailing `return s.GetAssetViewByID(ctx, assetID)` with a call to a new internal helper `getAssetWithLocationByID(ctx, id int) (*asset.AssetWithLocation, error)`.
- **New helper `getAssetWithLocationByID`:** mirrors `GetAssetByIdentifier` but keys on surrogate id — runs the same `SELECT ... FROM assets a LEFT JOIN locations l ...` query with `WHERE a.id = $1 AND a.deleted_at IS NULL LIMIT 1`, then fetches identifiers via `GetIdentifiersByAssetID`. Keep the helper unexported until a second caller exists. Both `UpdateAsset` and `CreateAssetWithIdentifiers` call this helper to assemble the rich view.

### `storage.UpdateLocation`
Same treatment as `UpdateAsset`: keep the UPDATE `RETURNING l.*` as-is, then call a new internal `getLocationWithParentByID` helper to fetch the self-joined parent natural key + identifiers. Returns `*location.LocationWithParent`.

### `storage.CreateLocationWithIdentifiers`
Same treatment as `CreateAssetWithIdentifiers`: tail call switches from `GetLocationViewByID` to the new `getLocationWithParentByID` helper (self-joins parent, fetches identifiers).

### Storage tests
Most existing tests compile unchanged — Go field promotion means `result.ID`, `result.Name`, `result.Identifier` etc. still resolve against the wrapping `*AssetWithLocation` / `*LocationWithParent`.
- Tests that compare the return value to a literal `*asset.Asset` / `*location.Location` (or type-assert on it) need to target the new type.
- Cross-org tests (`*_crossorg_test.go`) mostly assert nil/not-nil — no change expected.
- **New coverage:**
  - `TestUpdateAsset_PopulatesCurrentLocationIdentifier` — when `current_location_id` points at a live location, returned `CurrentLocationIdentifier` is non-nil and equal to that location's natural key. When nil, returned identifier is nil. And `Identifiers` slice is populated (not `nil`).
  - Twin for `TestUpdateLocation_PopulatesParentIdentifier`.

## Handler changes

### `backend/internal/handlers/assets/assets.go`

#### `Create` (line 86)
- **Delete** `createAssetWithoutIdentifiers` (lines 41-67).
- **Collapse** the if/else at lines 111-115 into a single call: `result, err := handler.storage.CreateAssetWithIdentifiers(r.Context(), request)`.
- `result` is now `*asset.AssetWithLocation`.
- `Location` header: `/api/v1/assets/` + `strconv.Itoa(result.ID)` — unchanged (surrogate id is on the embedded `Asset`).
- Response: `httputil.WriteJSON(w, http.StatusCreated, map[string]any{"data": asset.ToPublicAssetView(*result)})`.

#### `doUpdateAsset` (line 182)
- `result` type becomes `*asset.AssetWithLocation`; the nil check on line 207 still works.
- Replace line 213 with `httputil.WriteJSON(w, http.StatusOK, map[string]any{"data": asset.ToPublicAssetView(*result)})`.
- `UpdateAssetByID` already routes through `doUpdateAsset` — one fix corrects both the `/{identifier}` and `/by-id/{id}` endpoints.

#### Swagger typed envelopes
Alongside the existing `ListAssetsResponse` / `GetAssetResponse` (lines 326-336), add:

```go
type CreateAssetResponse struct {
    Data asset.PublicAssetView `json:"data"`
}

type UpdateAssetResponse struct {
    Data asset.PublicAssetView `json:"data"`
}
```

Update `@Success` annotations:
- `Create` (line 77): `@Success 201 {object} assets.CreateAssetResponse` (was `map[string]any`).
- `UpdateAsset` (line 141): `@Success 200 {object} assets.UpdateAssetResponse`.
- Drop `"data: asset.AssetView"` / `"data: asset.Asset"` description callouts.

### `backend/internal/handlers/locations/locations.go`

Mirror of the asset changes:
- Delete `createLocationWithoutIdentifiers` (lines 38-62).
- Collapse `Create` (line 81) to always call `CreateLocationWithIdentifiers`.
- `doUpdate` (line 173) projects via `ToPublicLocationView`.
- Add `CreateLocationResponse` / `UpdateLocationResponse` typed envelopes, update `@Success` annotations.

### Integration tests
In `assets_integration_test.go`, `by_id_integration_test.go`, and the location twins:
- Any create/update response assertion that reads `id`, `org_id`, or `current_location_id` / `parent_location_id` off the JSON swaps to `surrogate_id` and `current_location` / `parent` (the natural-key fields).
- Response-shape decoders typed to `asset.Asset` / `asset.AssetView` / `location.Location` / `location.LocationView` swap to `asset.PublicAssetView` / `location.PublicLocationView`.
- Drop any assertion that depends on the absent internal fields (e.g. asserting `org_id` round-trips).
- **New negative test per resource:** POST / PUT response body MUST NOT contain `id` or `org_id` keys. Belt-and-suspenders against regression.

## Frontend changes

### `frontend/src/lib/location/normalize.ts` (new)

Mirror `lib/asset/normalize.ts`:

```ts
import type { Location } from '@/types/locations';

export function normalizeLocation(raw: any): Location {
  const id = raw.id ?? raw.surrogate_id;
  return {
    ...raw,
    id,
    surrogate_id: raw.surrogate_id ?? raw.id,
  };
}
```

Plus colocated `normalize.test.ts` covering:
- Public shape (only `surrogate_id`) → `id` populated from `surrogate_id`.
- Legacy shape (only `id`) → `surrogate_id` populated from `id`.
- Both present → identity.

### `AssetFormModal.tsx`

Refactor so normalize happens once, immediately after each API call. All subsequent reads flow off the normalized object.

After `assetsApi.create(...)` (line 61) and after `assetsApi.update(...)` (line 108):

```ts
const raw = response.data?.data;
if (!raw || typeof raw !== 'object') {
  throw new Error('Invalid response from server. Asset API may not be available.');
}
const normalized = normalizeAsset(raw);
if (!normalized.id) {
  throw new Error('Invalid response from server. Asset API may not be available.');
}
```

Replace downstream reads:
- `response.data.data.id` (lines 67, 110) → `normalized.id`.
- `response.data.data.identifier` (toasts, lines 98, 137) → `normalized.identifier`.
- `normalizeAsset(response.data.data)` cache writes (lines 92, 95, 134) → `normalized` (or the separately-fetched `freshResponse` normalized, which already happens).

Behavior unchanged under the legacy shape because `normalizeAsset` is a no-op when `id` is already present.

### `LocationFormModal.tsx`

Identical pattern:
- Import `normalizeLocation`.
- Normalize once after `locationsApi.create(...)` (line 34) and after `locationsApi.update(...)` (line 74).
- Replace `response.data.data.id` / `response.data.data.identifier` reads (lines 36, 40, 57, 65, 76, 98) with references to `normalized`.
- `addLocation(...)` / `updateLocation(...)` cache writes (lines 57, 59, 62, 93, 95) receive `normalized` (or `normalizeLocation(freshResponse.data.data)` on the refetch path).

### Broader frontend audit
- **Asset hooks** (`hooks/assets/useAssets.ts`, `useAsset.ts`, `useAssetMutations.ts`) already import `normalizeAsset`. Expected to be no-op; verify via `pnpm test` and `pnpm typecheck`.
- **Location hooks** (`hooks/locations/useLocationMutations.ts`, `useLocations.ts`, `useLocation.ts`) reference `.id` on API responses without a normalize helper today. Wire `normalizeLocation` at any seam where an API response flows into cache/store.
- `frontend/src/lib/api/assets/assets.test.ts` asserts `.id` on a mocked response — the mock defines the shape, so the test is internally self-consistent. Leave it unless the corresponding assertion semantically changes.
- `hooks/locations/useLocationMutations.test.ts` — update only if its mocked responses or assertions reflect the old shape; if they target the normalize seam, align them with the new helper.

## Error handling

No new error classes or branches. Success-path projection is the only change; existing mappings (unique violation → 409, FK violation via `RespondStorageError`, nil → 404) all survive. The extra `GetIdentifiersBy*ID` round-trip in storage can fail — it already can in `GetAssetViewByID` today, and propagates as a 500, so no new handling.

## Edge cases

- **`current_location_id` nil.** LEFT JOIN yields nil `l.identifier`; `ToPublicAssetView` emits `current_location` as null (omitempty). Matches read-endpoint behavior today.
- **Update nulls the FK.** Same as above.
- **Legacy-shape responses reaching the frontend** (e.g., stale mock, cached response). `normalizeAsset` / `normalizeLocation` treat them as already-normalized; no breakage.
- **Surrogate-ID cache key.** `PublicAssetView` / `PublicLocationView` both include `surrogate_id` (per TRA-210), so `PUT /by-id/{id}` responses carry the frontend's cache key. No regression on session-auth write paths.

## Rollout

- Backend + frontend ship in the same PR → same preview deploy (`app.preview.trakrf.id` per `.github/workflows/sync-preview.yml`). Production merge is atomic.
- No database migration.
- No feature flag — atomic response-shape change, no external consumer locked in (TeamCentral has not integrated; the public contract is still in the v1 design window).

## Out of scope

- Bulk-import (`POST /api/v1/assets/bulk`) response shape — separate public surface, deferred per v1 scope.
- Renaming `surrogate_id`. Keep the field name as-is.
- Removing `normalizeAsset` / `normalizeLocation`. They stay as defense-in-depth; cost is zero when `id` is already present, and they protect against any unaudited read path that might still emit legacy shape.

## Verification gate

Before opening the PR:
- `just backend test` clean.
- `just backend lint` clean.
- `just frontend test` clean.
- `just frontend typecheck` clean.
- Preview deploy sanity check: create + edit one asset and one location via the UI. Network-tab payloads on create/update contain `surrogate_id` and omit `id` / `org_id`. No console errors.

## Related project context

- **TeamCentral** is the first public-API customer and hasn't integrated. The contract change is safe now; it will not be safe later.
- **Public API surfaces logical data only.** `surrogate_id` stays as an internal perf detail exposed only because the frontend's cache needs a stable key. Natural identifiers remain the primary key on public surfaces.
- **No squash merges** — this ships as a merge commit per project convention.
