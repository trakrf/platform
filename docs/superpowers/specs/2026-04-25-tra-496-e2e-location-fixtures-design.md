# TRA-496 — E2E Location Fixtures: Public-API Natural Keys

## Problem

The frontend e2e fixture `frontend/tests/e2e/fixtures/location.fixture.ts` and the inline helper in `frontend/tests/e2e/locations-after-login.spec.ts` were written before TRA-447 reshaped the public locations API. They still post `parent_location_id` derived from `result.data.id`, but the public response no longer returns `id`, `org_id`, or `parent_location_id`. Result against `gke.trakrf.app` on 2026-04-24: `warehouseA.id === undefined` → child posts go out as `parent_location_id: null` → all 8 locations land as roots → ~25 tests in the locations cluster fail (Total Locations: 8 / Root Locations: 8).

Same shape as TRA-487 (backend factories), but on the frontend e2e fixtures.

## Goals

- Restore the locations-* e2e cluster (~25 tests) to green against `gke.trakrf.app`.
- Align the e2e fixture with the public API contract an external SDK consumer would see — natural keys (identifiers) only, no surrogate IDs.
- Keep the change focused: two files, no unrelated refactoring.

## Non-Goals

- Consolidating `locations-after-login.spec.ts`'s inline `createTestLocation` into the shared fixture (different bug, TRA-318 territory).
- Migrating the internal `/locations/by-id/{id}` route off the frontend app code (TRA-425 territory; out of scope).
- Backend changes — TRA-447 already shipped.
- Per-ticket black-box verification — series-level batched per project policy.

## Design

### Architecture

Two files change:

- **`frontend/tests/e2e/fixtures/location.fixture.ts`** — sole owner of public-API location helpers shared across `locations-desktop.spec.ts`, `locations-mobile.spec.ts`, `locations-accessibility.spec.ts`.
- **`frontend/tests/e2e/locations-after-login.spec.ts`** — TRA-318 logout/login scenario; has its own inline `createTestLocation`. Patched in place; not extracted.

No new modules.

### Components

#### `CreateLocationData` (request shape)

```ts
interface CreateLocationData {
  identifier: string;
  name: string;
  description?: string;
  parent_identifier?: string | null;  // was: parent_location_id?: number | null
  is_active?: boolean;
}
```

#### `CreatedLocation` (response shape — trimmed `PublicLocationView`)

```ts
interface CreatedLocation {
  identifier: string;
  name: string;
  description: string;
  parent: string | null;     // parent identifier; was: parent_location_id: number | null
  path: string;
  depth: number;
  is_active: boolean;
}
```

- Drop `id` entirely. Do not include `surrogate_id` — fixture has no need to surface internal IDs (per "Surrogate IDs are internal perf detail" project memory).
- Drop `valid_from / valid_to / created_at / updated_at / identifiers` from the typed surface. The actual JSON response still contains them; the fixture's typed view is intentionally minimal to match what callers use.

#### `createLocationViaAPI`

- Body sends `parent_identifier` (string|null) instead of `parent_location_id`.
- Returns the trimmed `CreatedLocation`.

#### `createTestHierarchy`

Replace surrogate-ID-based parenting:

```ts
// before
parent_location_id: warehouseA.id

// after
parent_identifier: 'warehouse-a'
```

Identifiers are local string literals already in scope at each `createLocationViaAPI` call site, so we don't need to thread the parent's response back in.

#### `deleteAllLocationsViaAPI`

- Switch from `DELETE /api/v1/locations/by-id/{id}` (internal route) to `DELETE /api/v1/locations/{identifier}` (public route).
- Sort by `depth` desc (deepest first) instead of by presence of `parent_location_id`.
- Replace `deleteLocationViaAPI(page, id: number)` with `deleteLocationByIdentifierViaAPI(page, identifier: string)`.

#### `locations-after-login.spec.ts:35` `createTestLocation`

- Drop `id` from the `TestLocation` interface; return `{ identifier, name }`.
- Body unchanged (already sends `valid_from`, which the public POST accepts).
- Downstream tests reference `location.name` only, so dropping `id` is non-breaking.

### Data Flow

1. Test calls `createTestHierarchy(page)`.
2. For each location, fixture POSTs `{ identifier, name, parent_identifier?, ... }` to `/api/v1/locations`.
3. Backend resolves `parent_identifier` → internal `parent_location_id` (TRA-447 logic, `backend/internal/handlers/locations/locations.go:78`).
4. Response is `PublicLocationView`; fixture trims to its `CreatedLocation` shape (no IDs).
5. Children specify parent via the same string identifier already in local scope — no read-back of the parent's response.
6. Test runs assertions against the rendered tree.
7. Teardown: `deleteAllLocationsViaAPI` lists, sorts by `depth` desc, deletes each via `DELETE /locations/{identifier}`.

Parent and child creates remain sequential; no race-window concurrency change in this ticket.

### Error Handling

- **POST failure** → throws `Failed to create location: <status> - <body>`. No retries. Fast e2e signal.
- **DELETE failure during teardown** → caught and ignored (matches current behavior at `location.fixture.ts:136`). Reasoning: teardown runs against possibly-dirty state; cascade deletes may have removed children; we don't want teardown noise masking the real test failure.
- **GET failure during teardown** → throws. Without a list, teardown can't proceed and would leave the env wedged.
- **Parent-not-found case** (`parent_identifier` doesn't exist) → backend returns 422 with `{ field: "parent_identifier", ... }`. Surfaced as the standard `Failed to create location` throw. Indicates a test bug.

No new error paths introduced.

### Testing

Verification command (from Linear ticket):

```
PLAYWRIGHT_BASE_URL=https://gke.trakrf.app pnpm exec playwright test \
  --grep-invert "@hardware" tests/e2e/locations-*.spec.ts
```

Expected outcome:

- ~25 currently-failing locations-cluster tests pass.
- "Total Locations: 8 / Root Locations: 8" failure mode disappears (replaced by 8/2).
- Tests outside the locations cluster continue passing (no regressions).

Pre-flight (cheaper) checks before pushing:

- `pnpm typecheck` — confirms `CreateLocationData` / `CreatedLocation` shape changes don't break callers.
- `pnpm lint` — formatting/imports.

Not doing:

- No unit test for the fixture (thin wrapper around `page.request`; only meaningful coverage is the e2e suite consuming it).
- No mocking — the fixture talks to a real backend by design.
- No per-ticket black-box pass; series-batched per project policy.

## References

- Linear: TRA-496
- Linear: TRA-487 (backend parallel)
- Linear: TRA-447 (created the natural-key contract)
- Linear: TRA-442 (UI-side parallel cleanup)
- Code: `backend/internal/handlers/locations/locations.go:78` (parent_identifier resolution)
- Code: `backend/internal/models/location/public.go` (`PublicLocationView` shape)
