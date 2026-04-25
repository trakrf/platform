# TRA-496: E2E Location Fixtures Natural-Key Migration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Repair the locations-* e2e cluster (~25 failing tests on `gke.trakrf.app`) by switching the frontend e2e fixture from surrogate-ID parenting to public-API natural-key (`parent_identifier`), and aligning `CreatedLocation` with the post-TRA-447 public response shape.

**Architecture:** Two files change. (1) `frontend/tests/e2e/fixtures/location.fixture.ts` — request/response interfaces, `createLocationViaAPI`, `createTestHierarchy`, `deleteAllLocationsViaAPI` switch to identifiers and the public `DELETE /locations/{identifier}` route. (2) `frontend/tests/e2e/locations-after-login.spec.ts:35` — drop reliance on `data.data.id` from the inline `createTestLocation` helper. No backend changes (TRA-447 already shipped). No new modules.

**Tech Stack:** TypeScript, Playwright, pnpm, Vite. Run from repo root via `pnpm` workspace scripts (`pnpm typecheck`, `pnpm lint`, `pnpm validate`) and `just frontend <cmd>` delegates per project CLAUDE.md.

---

## File Structure

**Modify:**
- `frontend/tests/e2e/fixtures/location.fixture.ts` — interfaces (`CreateLocationData` line 14, `CreatedLocation` line 25), `createLocationViaAPI` (line 47), `deleteLocationViaAPI` (line 80), `deleteAllLocationsViaAPI` (line 122), `createTestHierarchy` (line 170)
- `frontend/tests/e2e/locations-after-login.spec.ts` — `TestLocation` interface (line 27), `createTestLocation` (line 35)

**No files created or deleted.**

---

## Task 1: Update fixture interfaces

**Files:**
- Modify: `frontend/tests/e2e/fixtures/location.fixture.ts:14-32`

- [ ] **Step 1: Update `CreateLocationData` (request shape)**

Find:

```ts
/**
 * Location creation data
 */
export interface CreateLocationData {
  identifier: string;
  name: string;
  description?: string;
  parent_location_id?: number | null;
  is_active?: boolean;
}
```

Replace with:

```ts
/**
 * Location creation data (public-API write shape per TRA-447).
 * Parent is referenced by natural identifier, not surrogate ID.
 */
export interface CreateLocationData {
  identifier: string;
  name: string;
  description?: string;
  parent_identifier?: string | null;
  is_active?: boolean;
}
```

- [ ] **Step 2: Update `CreatedLocation` (response shape)**

Find:

```ts
/**
 * Created location response
 */
export interface CreatedLocation {
  id: number;
  identifier: string;
  name: string;
  description: string;
  parent_location_id: number | null;
  is_active: boolean;
}
```

Replace with:

```ts
/**
 * Trimmed view of the public PublicLocationView response (TRA-447).
 * Surrogate IDs are intentionally omitted — fixtures use natural identifiers
 * end-to-end so e2e tests exercise the same contract external SDK consumers see.
 */
export interface CreatedLocation {
  identifier: string;
  name: string;
  description: string;
  parent: string | null;
  path: string;
  depth: number;
  is_active: boolean;
}
```

## Task 2: Update `createLocationViaAPI` body

**Files:**
- Modify: `frontend/tests/e2e/fixtures/location.fixture.ts:47-75`

- [ ] **Step 1: Switch the POST body field**

Find:

```ts
    data: {
      identifier: data.identifier,
      name: data.name,
      description: data.description || '',
      parent_location_id: data.parent_location_id ?? null,
      is_active: data.is_active ?? true,
    },
```

Replace with:

```ts
    data: {
      identifier: data.identifier,
      name: data.name,
      description: data.description || '',
      parent_identifier: data.parent_identifier ?? null,
      is_active: data.is_active ?? true,
    },
```

The function signature and return statement do not change — `result.data` from the public response is structurally compatible with the trimmed `CreatedLocation` (extra JSON keys like `surrogate_id`, `valid_from`, `identifiers` remain in the runtime object but are not surfaced through the typed view).

## Task 3: Replace delete-by-id with delete-by-identifier

**Files:**
- Modify: `frontend/tests/e2e/fixtures/location.fixture.ts:77-94`

- [ ] **Step 1: Rename and re-target the single-delete helper**

Find:

```ts
/**
 * Delete a location via API
 */
export async function deleteLocationViaAPI(page: Page, id: number): Promise<void> {
  const baseUrl = getApiBaseUrl();
  const token = await getAuthToken(page);

  const response = await page.request.delete(`${baseUrl}/locations/by-id/${id}`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to delete location: ${response.status()} - ${text}`);
  }
}
```

Replace with:

```ts
/**
 * Delete a location via the public API (DELETE /locations/{identifier}).
 * Uses the natural-key route so fixtures don't depend on internal surrogate IDs.
 */
export async function deleteLocationByIdentifierViaAPI(
  page: Page,
  identifier: string
): Promise<void> {
  const baseUrl = getApiBaseUrl();
  const token = await getAuthToken(page);

  const response = await page.request.delete(
    `${baseUrl}/locations/${encodeURIComponent(identifier)}`,
    {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  );

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to delete location: ${response.status()} - ${text}`);
  }
}
```

`encodeURIComponent` guards against any future identifier value containing `/` or other reserved characters; current test identifiers are simple slugs.

## Task 4: Update `deleteAllLocationsViaAPI` to sort by depth and delete by identifier

**Files:**
- Modify: `frontend/tests/e2e/fixtures/location.fixture.ts:118-140`

- [ ] **Step 1: Replace the sort + delete loop**

Find:

```ts
/**
 * Delete all locations for clean test state
 * Deletes in reverse order (children first) to avoid parent constraint issues
 */
export async function deleteAllLocationsViaAPI(page: Page): Promise<void> {
  const locations = await getLocationsViaAPI(page);

  // Sort by depth (deepest first) to delete children before parents
  // Locations without parent_location_id are root (depth 0)
  const sortedByDepth = locations.sort((a, b) => {
    const depthA = a.parent_location_id ? 1 : 0;
    const depthB = b.parent_location_id ? 1 : 0;
    return depthB - depthA;
  });

  for (const location of sortedByDepth) {
    try {
      await deleteLocationViaAPI(page, location.id);
    } catch {
      // Ignore errors (may already be deleted via cascade)
    }
  }
}
```

Replace with:

```ts
/**
 * Delete all locations for clean test state.
 * Sorts by depth descending (deepest first) so children are deleted before
 * their parents, avoiding FK constraint failures.
 */
export async function deleteAllLocationsViaAPI(page: Page): Promise<void> {
  const locations = await getLocationsViaAPI(page);

  const sortedByDepth = [...locations].sort((a, b) => b.depth - a.depth);

  for (const location of sortedByDepth) {
    try {
      await deleteLocationByIdentifierViaAPI(page, location.identifier);
    } catch {
      // Cascade may have removed children already; teardown errors are
      // intentionally swallowed so they don't mask real test failures.
    }
  }
}
```

## Task 5: Update `createTestHierarchy` to use natural identifiers

**Files:**
- Modify: `frontend/tests/e2e/fixtures/location.fixture.ts:166-239`

- [ ] **Step 1: Replace surrogate-ID parenting with identifier strings**

Find:

```ts
/**
 * Create a complete test hierarchy
 * Returns all created locations for use in tests
 */
export async function createTestHierarchy(page: Page): Promise<TestHierarchy> {
  // Create root locations
  const warehouseA = await createLocationViaAPI(page, {
    identifier: 'warehouse-a',
    name: 'Warehouse A',
    description: 'Main warehouse facility',
  });

  const warehouseB = await createLocationViaAPI(page, {
    identifier: 'warehouse-b',
    name: 'Warehouse B',
    description: 'Secondary warehouse',
  });

  // Create Floor 1 and Floor 2 under Warehouse A
  const floor1 = await createLocationViaAPI(page, {
    identifier: 'floor-1',
    name: 'Floor 1',
    description: 'First floor',
    parent_location_id: warehouseA.id,
  });

  const floor2 = await createLocationViaAPI(page, {
    identifier: 'floor-2',
    name: 'Floor 2',
    description: 'Second floor',
    parent_location_id: warehouseA.id,
  });

  // Create sections under floors
  const sectionA = await createLocationViaAPI(page, {
    identifier: 'section-a',
    name: 'Section A',
    description: 'Storage section A',
    parent_location_id: floor1.id,
  });

  const sectionB = await createLocationViaAPI(page, {
    identifier: 'section-b',
    name: 'Section B',
    description: 'Storage section B',
    parent_location_id: floor1.id,
  });

  const sectionC = await createLocationViaAPI(page, {
    identifier: 'section-c',
    name: 'Section C',
    description: 'Storage section C',
    parent_location_id: floor2.id,
  });

  // Create storage area under Warehouse B
  const storageArea = await createLocationViaAPI(page, {
    identifier: 'storage-area',
    name: 'Storage Area',
    description: 'General storage',
    parent_location_id: warehouseB.id,
  });

  return {
    warehouseA,
    floor1,
    sectionA,
    sectionB,
    floor2,
    sectionC,
    warehouseB,
    storageArea,
  };
}
```

Replace with:

```ts
/**
 * Create a complete test hierarchy.
 * Parent linkage uses parent_identifier (TRA-447 natural-key contract);
 * the parent's identifier is a string literal already in scope, so we
 * never need to read back a surrogate ID from a previous response.
 */
export async function createTestHierarchy(page: Page): Promise<TestHierarchy> {
  const warehouseA = await createLocationViaAPI(page, {
    identifier: 'warehouse-a',
    name: 'Warehouse A',
    description: 'Main warehouse facility',
  });

  const warehouseB = await createLocationViaAPI(page, {
    identifier: 'warehouse-b',
    name: 'Warehouse B',
    description: 'Secondary warehouse',
  });

  const floor1 = await createLocationViaAPI(page, {
    identifier: 'floor-1',
    name: 'Floor 1',
    description: 'First floor',
    parent_identifier: 'warehouse-a',
  });

  const floor2 = await createLocationViaAPI(page, {
    identifier: 'floor-2',
    name: 'Floor 2',
    description: 'Second floor',
    parent_identifier: 'warehouse-a',
  });

  const sectionA = await createLocationViaAPI(page, {
    identifier: 'section-a',
    name: 'Section A',
    description: 'Storage section A',
    parent_identifier: 'floor-1',
  });

  const sectionB = await createLocationViaAPI(page, {
    identifier: 'section-b',
    name: 'Section B',
    description: 'Storage section B',
    parent_identifier: 'floor-1',
  });

  const sectionC = await createLocationViaAPI(page, {
    identifier: 'section-c',
    name: 'Section C',
    description: 'Storage section C',
    parent_identifier: 'floor-2',
  });

  const storageArea = await createLocationViaAPI(page, {
    identifier: 'storage-area',
    name: 'Storage Area',
    description: 'General storage',
    parent_identifier: 'warehouse-b',
  });

  return {
    warehouseA,
    floor1,
    sectionA,
    sectionB,
    floor2,
    sectionC,
    warehouseB,
    storageArea,
  };
}
```

## Task 6: Verify fixture compiles, then commit

**Files:**
- No edits in this task — verification only.

- [ ] **Step 1: Type-check the frontend workspace**

Run: `just frontend typecheck`
Expected: passes with no errors.

If errors mention residual `parent_location_id` or `id`/`parent_location_id` reads on `CreatedLocation`, return to Tasks 1-5 to find the missed call site (search the whole frontend tree, not just `tests/e2e/`).

- [ ] **Step 2: Lint the frontend workspace**

Run: `just frontend lint`
Expected: passes with no errors.

- [ ] **Step 3: Confirm no other consumers of the renamed delete helper**

Run: `grep -rn "deleteLocationViaAPI" frontend/`
Expected: no matches (the only definition was the one renamed in Task 3, and no other file imports it).

If there are matches, update each import + call site to `deleteLocationByIdentifierViaAPI(page, identifier)`. The fixture's own call from `deleteAllLocationsViaAPI` was already updated in Task 4.

- [ ] **Step 4: Commit the fixture changes**

```bash
git add frontend/tests/e2e/fixtures/location.fixture.ts
git commit -m "$(cat <<'EOF'
fix(tra-496): switch e2e location fixture to public-API natural keys

POST /api/v1/locations stopped returning id/parent_location_id after
TRA-447, so createTestHierarchy was building all 8 locations as roots
(warehouseA.id === undefined → parent_location_id: null). Switch the
fixture to parent_identifier on the write path, sort teardown by depth,
and delete via the public DELETE /locations/{identifier} route so the
fixture no longer depends on internal surrogate IDs.
EOF
)"
```

## Task 7: Update `locations-after-login.spec.ts` inline helper

**Files:**
- Modify: `frontend/tests/e2e/locations-after-login.spec.ts:27-61`

- [ ] **Step 1: Drop `id` from the `TestLocation` interface**

Find:

```ts
interface TestLocation {
  id: number;
  name: string;
}
```

Replace with:

```ts
interface TestLocation {
  identifier: string;
  name: string;
}
```

- [ ] **Step 2: Stop reading `data.data.id` in `createTestLocation`**

Find:

```ts
  const data = await response.json();
  return {
    id: data.data.id,
    name: data.data.name,
  };
}
```

Replace with:

```ts
  const data = await response.json();
  return {
    identifier: data.data.identifier,
    name: data.data.name,
  };
}
```

`data.data.identifier` is present in `PublicLocationView` and matches what we passed in (`LOC-${uniqueId()}`). Downstream tests in this file reference `location.name` only — they do not read `location.id`, so dropping it is non-breaking.

- [ ] **Step 3: Fix the debug console.log that reads `location.id`**

Line 139 logs `(ID: ${location.id})`, which becomes a typecheck error after Step 1 drops `id` from `TestLocation`.

Find:

```ts
      console.log(`[LocationsAfterLogin] Created location: ${location.name} (ID: ${location.id})`);
```

Replace with:

```ts
      console.log(`[LocationsAfterLogin] Created location: ${location.name} (identifier: ${location.identifier})`);
```

- [ ] **Step 4: Verify no other `.id` reads remain on the test-location object**

Run: `grep -n "\.id\b" frontend/tests/e2e/locations-after-login.spec.ts`
Expected: any remaining matches refer to DOM `id` attributes (e.g. `input#email`) or unrelated identifiers, not to `TestLocation.id` or `data.data.id` on the create response.

If a `testLocations[i].id` reference still exists, the test was relying on surrogate ID. Since the assertions in this file are all text-content-based ("Verify specific location names are visible"), any such reference is dead code — remove it or convert to `.identifier` if it's a debug log.

## Task 8: Verify and commit second file

**Files:**
- No edits in this task — verification only.

- [ ] **Step 1: Type-check the frontend workspace**

Run: `just frontend typecheck`
Expected: passes with no errors.

- [ ] **Step 2: Lint the frontend workspace**

Run: `just frontend lint`
Expected: passes with no errors.

- [ ] **Step 3: Run combined validate to be safe**

Run: `just frontend validate`
Expected: passes (typecheck + lint + unit tests; no e2e in this target).

- [ ] **Step 4: Commit**

```bash
git add frontend/tests/e2e/locations-after-login.spec.ts
git commit -m "$(cat <<'EOF'
fix(tra-496): drop surrogate-id reliance in locations-after-login fixture

Inline createTestLocation predates the shared fixture and read
data.data.id, which TRA-447 removed from the public response. Switch
the TestLocation type to (identifier, name) — downstream assertions are
text-based and don't read the dropped id field.
EOF
)"
```

## Task 9: Final verification

**Files:**
- No edits — verification + handoff prep.

- [ ] **Step 1: Confirm clean working tree**

Run: `git status`
Expected: `nothing to commit, working tree clean` on branch `miks2u/tra-496-e2e-location-fixtures-broken-by-tra-447-uses`.

- [ ] **Step 2: Confirm both expected commits exist**

Run: `git log --oneline origin/main..HEAD`
Expected: two commits (excluding the pre-existing brainstorm spec commit) with `fix(tra-496):` prefixes from Tasks 6 and 8.

- [ ] **Step 3: (Optional) Local e2e spot-check if backend is running locally**

Only if a local backend is running on `http://localhost:8080`:

Run: `just frontend test:e2e --grep "Tree Navigation"`
Expected: at least one previously-failing locations-desktop test now passes.

If no local backend, skip — black-box verification on `gke.trakrf.app` is batched at the series level per project policy.

- [ ] **Step 4: Note for the PR description**

After implementation completes, the eventual PR body should include the verification command from the spec for reviewers to optionally run:

```
PLAYWRIGHT_BASE_URL=https://gke.trakrf.app pnpm exec playwright test \
  --grep-invert "@hardware" tests/e2e/locations-*.spec.ts
```

Expect the locations cluster (~25 tests) back to green, with "Total Locations: 8 / Root Locations: 2" in the rendered tree (was 8/8 pre-fix).
