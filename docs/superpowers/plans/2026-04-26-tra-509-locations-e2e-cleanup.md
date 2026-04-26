# TRA-509 — locations-* e2e cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the 30 failing tests in the `locations-*` Playwright cluster surfaced by the 2026-04-26 black-box e2e pass against gke. Buckets 1, 2, 3 are fixed in this PR. Bucket 4 (auth/401) is split into a new ticket. Bucket 5 and `inventory-save:346` are triaged inline.

**Architecture:** Tests assert against UI text that only exists in `LocationDetailsModal`, not the `LocationDetailsPanel` they actually exercise. Fix is to add stable `data-testid` markers on both Panel and Modal so they share a test contract, then convert affected assertions to `getByTestId`. Mobile/tablet test setups also click a desktop-only nav element behind the hamburger — rote fixture fix applied to 4 sites.

**Tech Stack:** React 18 + TypeScript, Playwright (e2e), Vitest (unit), Tailwind. Repo is `trakrf/platform` monorepo; commands run from project root via `just frontend ...`.

**Spec:** `docs/superpowers/specs/2026-04-26-tra509-locations-e2e-cleanup-design.md`

---

## File Map

**Modify:**
- `frontend/src/components/locations/LocationDetailsPanel.tsx` — add testids; rename "Sub-locations" → "Direct Children"
- `frontend/src/components/locations/LocationDetailsModal.tsx` — add matching testids
- `frontend/tests/e2e/locations-desktop.spec.ts` — convert text→testid assertions; fix strict-mode violations
- `frontend/tests/e2e/locations-mobile.spec.ts` — apply hamburger fixture to 2 sites; convert :184 (or wherever the assertion lives now); strict-mode fixes
- `frontend/tests/e2e/locations-accessibility.spec.ts` — apply hamburger fixture to 2 sites
- (Possibly) `frontend/src/components/locations/LocationExpandableCard.tsx` — if mobile card needs its own type testid
- `frontend/src/components/locations/LocationDetailsPanel.test.tsx` — update assertions if rename breaks them

**Out of scope:** any Modal restructure; any Panel restructure beyond label rename + testids; Bucket 4 fix; `EnvironmentBanner` cosmetic.

---

## Task 1: Add type marker + rename children heading on LocationDetailsPanel

**Files:**
- Modify: `frontend/src/components/locations/LocationDetailsPanel.tsx:93-127, 178-182`

- [ ] **Step 1: Add `data-testid="location-type"` element to header**

In the header div (around lines 93–127), add a third `<p>` inside the inner `<div>` that sits next to the icon, after the name line:

```tsx
<div className="flex items-center gap-3">
  <Icon className="h-6 w-6 text-gray-500 dark:text-gray-400" />
  <div>
    <h2 className="text-xl font-semibold text-gray-900 dark:text-white">
      {location.identifier}
    </h2>
    <p className="text-sm text-gray-500 dark:text-gray-400">{location.name}</p>
    <p
      data-testid="location-type"
      className="text-xs text-gray-500 dark:text-gray-400 mt-0.5"
    >
      {isRoot ? 'Root Location' : 'Subsidiary Location'}
    </p>
  </div>
</div>
```

- [ ] **Step 2: Rename children heading + split into testid'd spans**

Replace the existing children heading at line 180–182:

```tsx
<h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-3">
  Sub-locations ({children.length})
</h3>
```

with:

```tsx
<h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-3">
  <span data-testid="direct-children-label">Direct Children</span>
  {' ('}
  <span data-testid="direct-children-count">{children.length}</span>
  {')'}
</h3>
```

- [ ] **Step 3: Run unit tests; fix any rename-driven breakage**

Run from project root:
```
just frontend pnpm test src/components/locations/LocationDetailsPanel.test.tsx
```

Expected: pass. If a unit test asserts on the literal `Sub-locations` string, update it to assert via the new testids OR the new `Direct Children` text. Re-run.

- [ ] **Step 4: Run typecheck**

```
just frontend pnpm typecheck
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/locations/LocationDetailsPanel.tsx frontend/src/components/locations/LocationDetailsPanel.test.tsx
git commit -m "feat(tra-509): add testids and rename heading on LocationDetailsPanel

Adds data-testid markers (location-type, direct-children-label,
direct-children-count) to give Panel and Modal a shared test contract.
Renames 'Sub-locations (N)' -> 'Direct Children (N)' to match Modal."
```

---

## Task 2: Add matching testids to LocationDetailsModal

**Files:**
- Modify: `frontend/src/components/locations/LocationDetailsModal.tsx:156-176`

- [ ] **Step 1: Add `data-testid="location-type"` to the existing type `<p>`**

At line 156–158, change:

```tsx
<p className="font-medium text-gray-900 dark:text-white">
  {isRoot ? 'Root Location' : 'Subsidiary Location'}
</p>
```

to:

```tsx
<p
  data-testid="location-type"
  className="font-medium text-gray-900 dark:text-white"
>
  {isRoot ? 'Root Location' : 'Subsidiary Location'}
</p>
```

- [ ] **Step 2: Add `data-testid="direct-children-count"` to stat-box count**

At line 162–164, change:

```tsx
<p className="font-medium text-gray-900 dark:text-white">{children.length}</p>
```

to:

```tsx
<p
  data-testid="direct-children-count"
  className="font-medium text-gray-900 dark:text-white"
>
  {children.length}
</p>
```

NOTE: this is the stat-box value (sibling of the small "Direct Children" label at line 162's parent), not the section header below.

- [ ] **Step 3: Add `data-testid="direct-children-label"` to section-header `<p>`**

Find the `<p className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Direct Children:</p>` at around line 174–176 (it's the section header above the actual children list, NOT the stat-box label at ~line 162). Change to:

```tsx
<p
  data-testid="direct-children-label"
  className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-2"
>
  Direct Children:
</p>
```

The stat-box label (`<p>Direct Children</p>` at line 162 area) stays untouched — it doesn't need a testid for our contract.

- [ ] **Step 4: Run typecheck**

```
just frontend pnpm typecheck
```

Expected: pass.

- [ ] **Step 5: Run any Modal unit tests**

```
just frontend pnpm test src/components/locations
```

Expected: pass. No expected breakage; we only added attributes.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/locations/LocationDetailsModal.tsx
git commit -m "feat(tra-509): add matching testids to LocationDetailsModal

Mirrors testids added to LocationDetailsPanel so Panel and Modal share
a stable test contract. No structural or behavioral changes."
```

---

## Task 3: Apply mobile-sidebar hamburger fixture to 4 sites (Bucket 1)

**Files:**
- Modify: `frontend/tests/e2e/locations-mobile.spec.ts:321, 393`
- Modify: `frontend/tests/e2e/locations-accessibility.spec.ts:292, 448`

The replacement block (already used at `locations-mobile.spec.ts:60-62`):

```ts
await page.click('[data-testid="hamburger-button"]');
await page.waitForSelector('[data-testid="hamburger-dropdown"]');
await page.click('[data-testid="hamburger-dropdown"] [data-testid="menu-item-locations"]');
await page.waitForTimeout(500);
```

- [ ] **Step 1: Replace `locations-mobile.spec.ts:321`**

Find the line `await page.click('text="Locations"');` inside the Mobile Expandable Cards `beforeEach`. Replace that single line plus the following `await page.waitForTimeout(500);` with the four-line hamburger sequence above.

- [ ] **Step 2: Replace `locations-mobile.spec.ts:393`**

Same replacement inside the Tablet Viewport `beforeEach`.

NOTE: the tablet viewport is 768×1024 (still mobile layout). The hamburger pattern still applies because the desktop sidebar is hidden below the 1024px breakpoint.

- [ ] **Step 3: Replace `locations-accessibility.spec.ts:292`**

Same replacement inside the Mobile a11y `beforeEach`.

- [ ] **Step 4: Replace `locations-accessibility.spec.ts:448`**

Same replacement inside the Touch Target Size `beforeEach`.

- [ ] **Step 5: Decide whether to lift to a helper**

If Bucket 5 triage (Task 7) is going to add more mobile-sidebar sites, lift to `frontend/tests/e2e/helpers/openMobileLocationsNav.ts`:

```ts
import type { Page } from '@playwright/test';

export async function openMobileLocationsNav(page: Page) {
  await page.click('[data-testid="hamburger-button"]');
  await page.waitForSelector('[data-testid="hamburger-dropdown"]');
  await page.click('[data-testid="hamburger-dropdown"] [data-testid="menu-item-locations"]');
  await page.waitForTimeout(500);
}
```

Threshold: 5+ sites lifts; 4 sites stays inline. Defer the decision to Task 7's outcome — for now, leave inline.

- [ ] **Step 6: Run typecheck on test files**

```
just frontend pnpm typecheck
```

Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add frontend/tests/e2e/locations-mobile.spec.ts frontend/tests/e2e/locations-accessibility.spec.ts
git commit -m "test(tra-509): use mobile hamburger fixture in 4 locations

Replaces page.click('text=\"Locations\"') (which targets the hidden
desktop sidebar on mobile/tablet) with the existing hamburger->dropdown
sequence. Cascades to ~17 previously-failing tests."
```

---

## Task 4: Convert desktop Bucket 2 assertions to testids

**Files:**
- Modify: `frontend/tests/e2e/locations-desktop.spec.ts:177, 217, 253`

- [ ] **Step 1: Re-read the test at `locations-desktop.spec.ts:160-180`**

Confirm that line 171 (`await expect(detailsPanel).toBeVisible();`) is asserting panel visibility — NOT a Modal-string assertion. If that's still the case, leave :171 alone. If something has shifted, treat it like :177 below.

- [ ] **Step 2: Replace assertion at `:177`**

Change:
```ts
await expect(detailsPanel.locator('text=Root Location')).toBeVisible();
```
to:
```ts
await expect(detailsPanel.getByTestId('location-type')).toHaveText('Root Location');
```

- [ ] **Step 3: Replace assertion at `:217`**

Change:
```ts
await expect(detailsPanel.locator('text=Direct Children')).toBeVisible();
```
to:
```ts
await expect(detailsPanel.getByTestId('direct-children-label')).toBeVisible();
```

- [ ] **Step 4: Replace assertion at `:253`**

Change:
```ts
await expect(detailsPanel.locator('text=Subsidiary Location')).toBeVisible();
```
to:
```ts
await expect(detailsPanel.getByTestId('location-type')).toHaveText('Subsidiary Location');
```

- [ ] **Step 5: Run typecheck**

```
just frontend pnpm typecheck
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add frontend/tests/e2e/locations-desktop.spec.ts
git commit -m "test(tra-509): use Panel testids for type and direct-children assertions

Replaces text-based locators on the details panel with getByTestId
against the new shared Panel/Modal contract."
```

---

## Task 5: Investigate mobile-card type rendering and convert `locations-mobile.spec.ts` assertion

**Files:**
- Read: `frontend/src/components/locations/LocationExpandableCard.tsx`
- Possibly modify: `frontend/src/components/locations/LocationExpandableCard.tsx`
- Modify: `frontend/tests/e2e/locations-mobile.spec.ts:~184-190` (assertion is `text=Subsidiary` around line 190)

- [ ] **Step 1: Read `LocationExpandableCard.tsx`**

Look for where it renders type info. The failing test expects `text=Subsidiary` (no "Location" suffix) to be visible after expanding a child card. Three possible findings:

1. **Card already renders "Subsidiary"/"Root"** (likely as a small label or badge). Just needs a testid for the assertion to scope properly.
2. **Card doesn't render type at all** — the test was always asserting against ambient page text. In this case, ADD a `data-testid="location-type"` element on the card with `Root`/`Subsidiary` text, parallel to Panel/Modal.
3. **Card renders type via icon only**, no text. Same fix as case 2.

- [ ] **Step 2: If case 2 or 3, add the testid element to LocationExpandableCard**

Add a `<span data-testid="location-type" className="text-xs text-gray-500">{isRoot ? 'Root' : 'Subsidiary'}</span>` (or "Root Location"/"Subsidiary Location" — pick whichever is consistent with what the visible Panel/Modal use; the test currently asserts just `Subsidiary` so the short form is the lower-friction path) inside the expanded card content.

If it's case 1 (already rendered), skip this step.

- [ ] **Step 3: Update the failing assertion**

The failing test is "should show Subsidiary type for child location" — the assertion at around line 190:

```ts
await expect(page.locator('text=Subsidiary')).toBeVisible();
```

Change to:

```ts
await expect(floor1Card.getByTestId('location-type')).toContainText('Subsidiary');
```

(`toContainText` so it works whether the testid renders `Subsidiary` or `Subsidiary Location`.)

If there's a parallel "should show Root Location type" test in the same describe (likely lines 168–174 area), apply the same testid pattern.

- [ ] **Step 4: Run typecheck**

```
just frontend pnpm typecheck
```

- [ ] **Step 5: Run unit tests for LocationExpandableCard**

```
just frontend pnpm test src/components/locations/LocationExpandableCard.test.tsx
```

Expected: pass. If the card got a new testid element and a unit test asserts on text presence, update it.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/locations frontend/tests/e2e/locations-mobile.spec.ts
git commit -m "test(tra-509): use testid for mobile card type assertion

If LocationExpandableCard didn't already render type text, adds a
data-testid='location-type' marker matching the Panel/Modal contract."
```

---

## Task 6: Bucket 3 — fix strict-mode locator violations

**Files:**
- Modify: `frontend/tests/e2e/locations-desktop.spec.ts` (find offenders by greppable text)
- Possibly modify: `frontend/tests/e2e/locations-mobile.spec.ts`

Known offenders from the ticket:
- `text=Warehouse A` matches 2 elements (identifier label + name display)
- `text=Move Location` matches 2 elements
- `text=test-location` matches 2 elements
- `text=Active` matches 5 elements

- [ ] **Step 1: Grep for each offender**

```
just frontend rg -n "text=Warehouse A|text=Move Location|text=test-location|text=Active" tests/e2e/
```

Expected: a list of source lines. Capture them for the next steps.

- [ ] **Step 2: For each match, scope or use `.first()`**

Preference order:
1. **Scope to existing testid:** if the assertion is meaningful on the panel specifically, wrap with `page.getByTestId('location-details-panel').getByText(...)`. If on a card or row, wrap with that container's testid.
2. **More specific role/label selector:** e.g. `page.getByRole('heading', { name: 'Warehouse A' })` for the panel name.
3. **`.first()`:** only if the assertion is genuinely "any of these matches" and the order is stable.

For `text=Active` (5 matches): the assertion almost certainly wants the panel's status badge. Scope to `detailsPanel.getByText('Active')` if a panel reference is in scope; otherwise `page.getByTestId('location-details-panel').getByText('Active')`.

For `text=Move Location`: this is likely the Move modal/dialog title or a button. If the assertion is "the Move Location dialog opened", scope to `page.getByRole('dialog').getByText('Move Location')` or use `page.getByRole('heading', { name: 'Move Location' })`.

For `text=Warehouse A` and `text=test-location`: scope to the panel or to a row's testid based on intent.

- [ ] **Step 3: Apply each fix**

Edit each line found in Step 1 according to the strategy from Step 2. No batch search/replace — each one needs intent-aware judgment.

- [ ] **Step 4: Run typecheck**

```
just frontend pnpm typecheck
```

- [ ] **Step 5: Commit**

```bash
git add frontend/tests/e2e/locations-desktop.spec.ts frontend/tests/e2e/locations-mobile.spec.ts
git commit -m "test(tra-509): scope text locators to fix strict-mode violations

Replaces ambiguous text= locators with testid-scoped or role-based
selectors. Uses .first() only where the assertion is order-agnostic."
```

---

## Task 7: Triage Bucket 5 (`locations-desktop:472`, `locations-mobile:354/361/389`)

**Files:**
- Read first; modify based on triage outcome.

- [ ] **Step 1: Read each test to capture intent**

Look at:
- `locations-desktop.spec.ts:472`
- `locations-mobile.spec.ts:354` (FAB create on mobile, fresh state)
- `locations-mobile.spec.ts:361` (FAB visibility on mobile, fresh state)
- `locations-mobile.spec.ts:389` (Tablet viewport assertion)

For each, identify whether the test was using `page.click('text="Locations"')` (B1 pattern), creating a location via API/FAB and getting 401 (B4 pattern), or hitting some third issue.

- [ ] **Step 2: Run each individually against preview**

After Tasks 1–6 are pushed (or via a temporary local push to a branch deploy), run:

```
PLAYWRIGHT_BASE_URL=https://app.preview.trakrf.id pnpm exec playwright test \
  --grep-invert "@hardware" \
  -g "should create new location via FAB" \
  tests/e2e/locations-mobile.spec.ts
```

(Substitute test names for each.)

Capture the actual failure message. Categorize each test:
- **B1 pattern** → fold into Task 3 (apply hamburger fixture to that test's `beforeEach`)
- **B4 / 401 path** → defer, will be referenced from the new B4 ticket
- **Other** → fix in this PR if obvious; otherwise note in commit/ticket

- [ ] **Step 3: Apply fixes for B1-categorized tests**

If any of these tests' `beforeEach` blocks still use `page.click('text="Locations"')`, replace with the hamburger sequence. If 5+ total sites have been touched across this task and Task 3, lift to `frontend/tests/e2e/helpers/openMobileLocationsNav.ts` (per Task 3 Step 5) and replace all call sites.

- [ ] **Step 4: Document deferred tests**

Note in commit message which B5 tests were deferred to the B4 split-out ticket. They will be referenced from that ticket's description.

- [ ] **Step 5: Commit**

```bash
git add frontend/tests/e2e/
git commit -m "test(tra-509): triage Bucket 5 fresh-state/FAB tests

Categorized each: <list which were folded into B1 vs deferred to B4
ticket vs other>. Applied B1-pattern fixes inline."
```

---

## Task 8: Triage `inventory-save.spec.ts:346`

**Files:**
- Possibly modify: `frontend/tests/e2e/inventory-save.spec.ts`

- [ ] **Step 1: Reproduce against preview**

```
PLAYWRIGHT_BASE_URL=https://app.preview.trakrf.id pnpm exec playwright test \
  --grep-invert "@hardware" \
  tests/e2e/inventory-save.spec.ts
```

Capture the failure: which assertion, what state, console output.

- [ ] **Step 2: Categorize**

- **Wait condition / selector tweak / fixture path issue** (~30min or less to fix) → piggyback fix in this PR
- **Real Save-button regression**, **org-switch logic bug**, or anything that needs a backend change → file a new Linear ticket, reference from this PR description, leave the test as-is

- [ ] **Step 3: If piggybacking, apply fix and commit**

```bash
git add frontend/tests/e2e/inventory-save.spec.ts
git commit -m "test(tra-509): fix inventory-save:346 <one-line cause>

Piggybacked because <reason fix was small>."
```

- [ ] **Step 4: If splitting, file ticket**

Use `mcp__linear-server__save_issue` with team `TrakRF`, project `Build TrakRF SaaS database MVP`, label `Bug`, link to TRA-509 in description. Leave test as-is in this PR; record the new ticket ID for the PR description.

---

## Task 9: Investigate Bucket 4 (`createTestLocation` 401), file new ticket

**Files:**
- Read: `frontend/tests/e2e/fixtures/location.fixture.ts` (or wherever `createTestLocation` lives)
- Read: `frontend/tests/e2e/locations-after-login.spec.ts` (the 2 failing tests at :136 and :240)

- [ ] **Step 1: Locate `createTestLocation` and read it**

```
just frontend rg -n "createTestLocation" tests/e2e/
```

Read the implementation. Note how it gets the auth token (storage, cookies, request context, header).

- [ ] **Step 2: Run a failing test against preview with debug logging**

Add a temporary console.log of the auth header and login response timing in the fixture (DO NOT commit this), then run:

```
PLAYWRIGHT_BASE_URL=https://app.preview.trakrf.id pnpm exec playwright test \
  -g "TRA-318" \
  tests/e2e/locations-after-login.spec.ts
```

Capture output: is the token present? Does the 401 body indicate "expired", "invalid signature", or "missing"?

- [ ] **Step 3: Categorize**

- **Token malformed / missing** → fixture bug (test-side ticket)
- **Token valid on wire but rejected** → backend/auth bug (backend ticket)
- **Race (token in storage but request fires too soon)** → fixture timing bug (test-side ticket)

- [ ] **Step 4: Remove debug logging**

Revert the temporary console.logs from the fixture.

- [ ] **Step 5: File new Linear ticket**

Use `mcp__linear-server__save_issue` with:
- Team: `TrakRF`
- Project: `Build TrakRF SaaS database MVP`
- Title: appropriate for the category (e.g., "createTestLocation fixture: 401 on POST /api/v1/locations after fresh login" for fixture-side, or "POST /api/v1/locations rejects valid token from fresh login" for backend)
- Labels: `frontend` + `Bug` for fixture; `backend` + `Bug` for backend
- Description: include the captured evidence from Step 2, link to TRA-509, list any Bucket 5 tests deferred to this ticket per Task 7
- Use `state` parameter, NOT `status` (Linear MCP gotcha — see memory)

Note the new ticket ID (e.g., TRA-510-ish) for the PR description.

---

## Task 10: Local validation

**Files:** none.

- [ ] **Step 1: Typecheck**

```
just frontend pnpm typecheck
```
Expected: pass.

- [ ] **Step 2: Lint**

```
just frontend pnpm lint
```
Expected: pass.

- [ ] **Step 3: Unit tests**

```
just frontend pnpm test
```
Expected: pass. If anything fails, return to the offending task.

---

## Task 11: Push branch and open PR

**Files:** none.

- [ ] **Step 1: Push branch**

```bash
git push -u origin miks2u/tra-509-e2e-locations-cleanup
```

- [ ] **Step 2: Open PR**

```bash
gh pr create --title "fix(tra-509): locations-* e2e cleanup (B1+B2+B3, B4 split-out)" --body "$(cat <<'EOF'
## Summary
- B1: applied mobile hamburger fixture to 4 sites (~17 tests)
- B2: added shared testid contract on LocationDetailsPanel/Modal; converted desktop and mobile assertions
- B3: scoped strict-mode locator violations
- B5: triaged — <fold/defer counts>
- B4 (`createTestLocation` 401, 2 tests) split out to <NEW_TICKET_ID>
- inventory-save:346 — <piggybacked / split to NEW_TICKET_ID>

## Test plan
- [ ] CI green on preview deploy
- [ ] `PLAYWRIGHT_BASE_URL=https://app.preview.trakrf.id pnpm exec playwright test --grep-invert "@hardware" tests/e2e/locations-*.spec.ts` shows locations cluster green
- [ ] Bucket 4 tests still failing (by design, tracked in <NEW_TICKET_ID>)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Substitute `<NEW_TICKET_ID>` and `<fold/defer counts>` with the actuals from Tasks 7, 8, 9.

---

## Task 12: Verify against preview

**Files:** none (or remediation in earlier task files if anything fails).

- [ ] **Step 1: Wait for preview deploy**

Wait for the GitHub `sync-preview` workflow to deploy the PR to `https://app.preview.trakrf.id`. Watch via `gh pr checks <PR>` or the Actions tab.

- [ ] **Step 2: Run affected e2e specs against preview**

```
cd /home/mike/platform/.worktrees/tra-509
PLAYWRIGHT_BASE_URL=https://app.preview.trakrf.id pnpm exec playwright test \
  --grep-invert "@hardware" \
  tests/e2e/locations-*.spec.ts
```

Plus, only if Task 8 piggybacked the fix:
```
PLAYWRIGHT_BASE_URL=https://app.preview.trakrf.id pnpm exec playwright test \
  --grep-invert "@hardware" \
  tests/e2e/inventory-save.spec.ts
```

Expected:
- All B1, B2, B3 tests green
- B4 tests still failing (`locations-after-login.spec.ts:136,240`)
- B5 tests either green (folded into B1) or in the deferred set referenced by the new ticket

- [ ] **Step 3: Iterate on any unexpected failures**

If a green-expected test still fails: read the failure, identify the bucket, fix, push, re-run. If it's truly a new bug, file a separate ticket and proceed.

- [ ] **Step 4: Confirm done criteria**

- All B1, B2, B3 tests green ✓
- B5 tests handled per Task 7's outcome ✓
- `inventory-save:346` handled per Task 8's outcome ✓
- B4 ticket exists and is referenced from PR description ✓
- Typecheck, lint, unit tests passing locally ✓

- [ ] **Step 5: Hand back to user for merge**

PR is ready for review/merge. Per the project's "always PR, never merge locally" rule and "no squash merges" preference, the user will merge via GitHub UI using a merge commit.
