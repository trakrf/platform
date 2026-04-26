# TRA-509 — locations-* e2e cleanup (post-TRA-496)

**Linear:** [TRA-509](https://linear.app/trakrf/issue/TRA-509/e2e-findings-suite-cleanup-30-locations-fails-after-tra-496)
**Branch:** `miks2u/tra-509-e2e-locations-cleanup`
**Date:** 2026-04-26

## Background

Black-box e2e pass against `gke.trakrf.app` on 2026-04-26 (against `0a091de`) showed 30 fails / 137 pass / 4 skip / 6 did-not-run. All 30 remaining fails are in the `locations-*` test cluster and bucket cleanly into 5 categories. Bucket details and root causes are in the Linear ticket; this spec focuses on the chosen fixes and scope.

## Scope

### In scope (this PR)

- **Bucket 1** — apply existing mobile-sidebar hamburger fixture pattern to 4 sites in `locations-mobile.spec.ts` and `locations-accessibility.spec.ts`. Cascades to ~17 tests.
- **Bucket 2** — add stable testids to `LocationDetailsPanel.tsx` and `LocationDetailsModal.tsx` so Panel and Modal share a test contract; render `Root Location`/`Subsidiary Location` and `Direct Children (N)` on the Panel; update Panel-asserting tests to `getByTestId`. ~5 tests.
- **Bucket 3** — replace ambiguous text locators with testid-scoped selectors (preferred) or `.first()` where a testid isn't meaningful. ~5 tests.

### Triage during implementation, then fold or split

- **Bucket 5** (fresh-state/FAB, ~4–5 tests): triage each.
  - If it's `text="Locations"` against hidden desktop sidebar → fold into Bucket 1
  - If it's `POST /api/v1/locations` 401 → defer to the B4 split-out ticket
  - If something else → fix here, note in commit
- **`inventory-save.spec.ts:346`**: reproduce against preview.
  - If fix is < ~30 min → piggyback
  - If real Save-button or org-switch bug → file a new ticket, reference from this PR

### Out of scope (separate ticket)

- **Bucket 4** — `createTestLocation` returns 401 (2 tests). Investigation captures the request/response evidence, then files a fixture-side or backend ticket as appropriate. We do NOT fix Bucket 4 in this PR; we only categorize it correctly.

## Component changes

### `LocationDetailsPanel.tsx`

**Header (around lines 93–127)** — add a small type label under the existing identifier/name lines, right of the icon:

```tsx
<div className="flex items-center gap-3">
  <Icon className="h-6 w-6 ..." />
  <div>
    <h2 ...>{location.identifier}</h2>
    <p ...>{location.name}</p>
    <p data-testid="location-type" className="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
      {isRoot ? 'Root Location' : 'Subsidiary Location'}
    </p>
  </div>
</div>
```

**Children heading (around lines 178–182)** — keep visible shape `Direct Children (N)`, split into testid'd spans:

```tsx
<h3 className="text-lg font-semibold ...">
  <span data-testid="direct-children-label">Direct Children</span>
  {' ('}
  <span data-testid="direct-children-count">{children.length}</span>
  {')'}
</h3>
```

This renames the heading from `Sub-locations (N)` → `Direct Children (N)` to align with the Modal's wording.

### `LocationDetailsModal.tsx`

Add the matching testids to existing markup — no structural change:

- `data-testid="location-type"` on the `<p>{isRoot ? 'Root Location' : 'Subsidiary Location'}</p>` (around line 156–158)
- `data-testid="direct-children-label"` on the **section-header** `<p>Direct Children:</p>` above the children list (around line 174–176). Trailing colon stays — the testid is the contract. NOT the stat-box label at ~line 162; that one stays untouched.
- `data-testid="direct-children-count"` on the stat-box children-count `<p>{children.length}</p>` (around line 162–164)

Out of scope for this ticket: redesigning either component, adding a "Hierarchy Information" stat block to the Panel, or removing the "Total Descendants" stat from the Modal.

## Test changes

### Bucket 1 — mobile sidebar fixture (4 sites)

Replace `await page.click('text="Locations"')` with the hamburger sequence from `locations-mobile.spec.ts:60-62` at:
- `locations-mobile.spec.ts:321` (Mobile Expandable Cards `beforeEach`)
- `locations-mobile.spec.ts:393` (Tablet Viewport `beforeEach`)
- `locations-accessibility.spec.ts:292` (Mobile `beforeEach` — cascades to 8 tests)
- `locations-accessibility.spec.ts:448` (Touch Target Size `beforeEach`)

Replacement block:
```ts
await page.click('[data-testid="hamburger-button"]');
await page.waitForSelector('[data-testid="hamburger-dropdown"]');
await page.click('[data-testid="hamburger-dropdown"] [data-testid="menu-item-locations"]');
await page.waitForTimeout(500);
```

If Bucket 5 triage adds more sites, lift to a helper (`openMobileLocationsNav(page)` in `tests/e2e/helpers/`). 4 sites is the threshold — 5+ sites lifts; 4 stays inline. Cheap to revert either way.

### Bucket 2 — Panel/Modal text → testid

- `locations-desktop.spec.ts:177` (`text=Root Location`) → `getByTestId('location-type')` with text assertion
- `locations-desktop.spec.ts:217` (`text=Direct Children`) → `getByTestId('direct-children-label')`
- `locations-desktop.spec.ts:253` (`text=Subsidiary Location`) → `getByTestId('location-type')` with text assertion
- `locations-mobile.spec.ts:184` → same pattern as desktop:177 (verify line during implementation)
- `locations-desktop.spec.ts:171` (panel content for selected root) → re-read at impl time; convert to testid if it asserts the same content; leave otherwise

### Bucket 3 — strict-mode locator violations (~5 tests)

Triage each. Preference order:
1. Scope by existing testid (e.g. `page.getByTestId('location-details-panel').getByText(...)`)
2. More specific role/label selector
3. `.first()` only when the assertion is genuinely "any of these matches"

Known offenders from the ticket: `text=Warehouse A` (2 hits), `text=Move Location` (2 hits), `text=test-location` (2 hits), `text=Active` (5 hits). Most should resolve via Panel- or row-scoped queries.

## Triage protocol — implementation-time decisions

Each item below has a defined exit criterion so we don't drift.

### Bucket 5 (`locations-desktop:472`, `locations-mobile:354/361/389`)

Run each test individually against preview, read the failure mode:
- `text="Locations"` against hidden desktop sidebar → fold into Bucket 1
- `POST /api/v1/locations` 401 → fold into the Bucket 4 split-out ticket
- Else → fix in this PR, note category in commit message

Exit: every B5 test has been categorized; ones in this PR are fixed; rest are referenced from the new ticket.

### `inventory-save.spec.ts:346`

Reproduce against preview.
- < ~30 min fix (wait condition, selector tweak, fixture path) → piggyback
- Real Save-button regression or org-switch logic bug → file new ticket, reference from this PR's description, leave test as-is

Exit: failure root-caused; either fixed here or filed elsewhere.

### Bucket 4 split-out

Investigate just enough to file the right ticket:
- Capture: auth header, login response, 401 body
- Token malformed/missing → fixture bug (test-side ticket)
- Token valid on wire but rejected → backend/auth bug (backend ticket)
- Race (token in storage but not yet sent) → fixture timing bug (test-side ticket)

Exit: new Linear ticket exists, linked from TRA-509, with captured evidence. Bucket 4 is NOT fixed in this PR.

## Verification

### Local (in worktree)
```
just frontend pnpm typecheck
just frontend pnpm lint
just frontend pnpm test
```
Plus a static read of every test edit before commit.

### On PR (CI + preview)
- PR open auto-deploys to `https://app.preview.trakrf.id`
- CI runs affected specs against preview:
  ```
  PLAYWRIGHT_BASE_URL=https://app.preview.trakrf.id pnpm exec playwright test \
    --grep-invert "@hardware" \
    tests/e2e/locations-*.spec.ts
  ```
  Add `tests/e2e/inventory-save.spec.ts` only if `:346` was piggybacked.
- Expected: locations cluster green; B4 (+ any B5 routed there) still failing by design.

### Post-merge
Next deploy to gke + the next batch black-box e2e run is the production-like confirmation. Out of scope for this PR.

## Done criteria

- All Bucket 1, Bucket 2, Bucket 3 tests green against preview
- Bucket 5 tests either green (folded into B1) or explicitly referenced in the Bucket 4 split-out ticket with failure category recorded
- `inventory-save:346` either green (piggybacked) or referenced in a new ticket
- `pnpm typecheck`, `pnpm lint`, `pnpm test` all pass
- PR description links Bucket 4 split-out ticket and any additional split-outs

## Non-goals

- Redesigning `LocationDetailsPanel` or `LocationDetailsModal`
- Adding a "Hierarchy Information" stat block to the Panel
- Refactoring location store or hierarchy fetch logic
- Fixing the `EnvironmentBanner` "Preview Environment" cosmetic on gke (separate ticket)
- Fixing Bucket 4 (`createTestLocation` 401)
