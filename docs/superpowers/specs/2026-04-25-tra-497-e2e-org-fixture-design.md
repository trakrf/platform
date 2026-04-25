# TRA-497 — E2E org-crud fixture: stable switcher signal + correct listing assertion

**Status:** Brainstorm
**Branch:** `worktree-tra-497` (will be renamed to `miks2u/tra-497-e2e-org-crud-helpers-fail-switcher-button-text-and-personal` for the PR)
**Linear:** https://linear.app/trakrf/issue/TRA-497

## Problem

A black-box e2e run against `gke.trakrf.app` on 2026-04-24 surfaced 6 failing tests in `frontend/tests/e2e/org-crud.spec.ts`, all rooted in two test-side bugs:

1. **`switchToOrg` waits for impossible text.** The helper at `frontend/tests/e2e/fixtures/org.fixture.ts:111-114` waits for the switcher button to contain the org name as visible text. The button (`OrgSwitcher.tsx:62-75`) only renders a single-letter avatar (the user's email initial). The selector cannot match by construction; every call hits the 10s timeout.

2. **Listing test asserts a non-existent menuitem.** `org-crud.spec.ts:115` asserts `page.getByRole('menuitem', { name: testEmail })` is visible. The product never creates a "personal org named after the user's email" — the dropdown only lists `org.name` values. The test is testing for behavior that doesn't exist in the current product.

A third bug is adjacent: `org-crud.spec.ts:42` constructs `testOrgName = \`Test Org ${id}\`` but never passes it to `signupTestUser` (line 47). The signup helper falls back to its own unique default. The variable is dead, which is why the listing test reaches for `testEmail` — there's no other deterministic name to assert on.

The TRA-497 issue description also lists `inventory-save.spec.ts:346 › switching orgs clears asset mappings`. That test uses `switchOrgViaAPI` (not the broken UI helper) and is misattributed; it is **out of scope** for this ticket and gets a separate ticket if it remains red after this fix.

## Goals

- Restore green for all 6 failing `org-crud.spec.ts` tests against `gke.trakrf.app`.
- Give E2E tests a model-truth signal for "current org changed" that doesn't depend on the rendered text of the switcher button.
- Keep changes scoped to the two affected files plus a one-line product affordance. No public API changes, no UX changes.

## Non-goals

- Changing what the switcher button renders. Showing the org name on the button is reasonable UX but is scope creep here — kept as a separate consideration.
- Migrating callers to API-based org switching. `switchToOrg` (UI flow) and `switchOrgViaAPI` (API flow) both already exist; tests pick the one they want. Six callers stay on the UI helper.
- Backfilling unit-test coverage for the new data attributes. The e2e tests that consume them are the coverage.

## Approach

Three coordinated changes — one product affordance, two test fixes:

1. **Product (`OrgSwitcher.tsx`):** Expose `data-current-org-id` and `data-current-org-name` on the existing `Menu.Button`. Both values already live in `useOrgStore().currentOrg`; this is two attribute lines, no new state.
2. **Test fixture (`org.fixture.ts`):** Rewrite `switchToOrg` to wait on `data-current-org-name` instead of the impossible `:has-text()` selector. Switch the click locator from `button:has-text(...)` to `getByRole('menuitem', {name})`.
3. **Test specs (`org-crud.spec.ts`):** Pass `testOrgName` into `signupTestUser` so the variable becomes load-bearing. Replace the `testEmail` menuitem assertion with `testOrgName`.

### Why expose both id and name attributes

The cost is one line each; both values are already on the component's state. Tests holding a name (today's `switchToOrg(page, orgName)` callers) wait on `data-current-org-name`. Tests holding an id (anything that called `createOrgViaAPI` and got `{id}` back) can wait on `data-current-org-id`. Future tests get a free choice between natural-key and surrogate-id lookup, consistent with the rest of the test fixtures already mixing UI and API helpers.

### Why drop the explicit menu-hidden wait

The current helper's `page.waitForSelector('[role="menu"]', { state: 'hidden' })` is a redundant intermediate signal. Headless UI unmounts the menu after a click, and the new `data-current-org-name` attribute settle is a stronger signal: the store updated, the component re-rendered, the attribute now reflects the new org — which can only happen after the dropdown closes and the switch completed.

## Affected files

```
frontend/src/components/OrgSwitcher.tsx          — add 2 data attributes
frontend/tests/e2e/fixtures/org.fixture.ts       — rewrite switchToOrg (lines 104-115)
frontend/tests/e2e/org-crud.spec.ts              — pass testOrgName to signup, swap menuitem name
```

## Detailed changes

### 1. `frontend/src/components/OrgSwitcher.tsx`

Add two attributes to the existing `Menu.Button`:

```tsx
<Menu.Button
  disabled={isLoading}
  data-testid="org-switcher"
  data-current-org-id={currentOrg?.id ?? ''}
  data-current-org-name={currentOrg?.name ?? ''}
  className="..."
>
```

Empty string (rather than omitting the attribute) when there is no current org — keeps the DOM shape consistent so tests can wait on a non-empty value as an implicit "loaded" signal. Visible behavior unchanged.

### 2. `frontend/tests/e2e/fixtures/org.fixture.ts`

Replace the body of `switchToOrg` (lines 104-115) with:

```ts
export async function switchToOrg(page: Page, orgName: string): Promise<void> {
  await openOrgSwitcher(page);
  // Click the org in the dropdown menu
  await page.getByRole('menuitem', { name: orgName }).click();

  // Wait for the switcher button to reflect the new current org via its
  // data-current-org-name attribute — model truth, not rendered text.
  await page.waitForSelector(
    `[data-testid="org-switcher"][data-current-org-name="${orgName}"]`,
    { timeout: 10000 }
  );
}
```

Removes:
- The `[role="menu"] state: hidden` intermediate wait (subsumed by the attribute settle).
- The `[data-testid="org-switcher"]:has-text(...)` selector that cannot match.

Changes the click locator from `button:has-text(...)` to `getByRole('menuitem', {name})` for precision and to align with how the listing test already finds menuitems.

### 3. `frontend/tests/e2e/org-crud.spec.ts`

**3a.** Wire `testOrgName` into signup (line 47):

```ts
await signupTestUser(page, testEmail, testPassword, testOrgName);
```

**3b.** Replace the email-named menuitem assertion (line 115):

```ts
// Should see the org from signup
await expect(page.getByRole('menuitem', { name: testOrgName })).toBeVisible();
```

No other changes. The Edit/Delete tests (Edit Test Org, Empty Edit Org, delete-confirmation-modal, exact-name-match-to-delete, delete-and-redirect-to-home) already call `switchToOrg(page, orgName)` and become green once the helper is fixed.

## Verification

The full e2e cluster against gke (mirrors the original failure environment):

```bash
PLAYWRIGHT_BASE_URL=https://gke.trakrf.app pnpm exec playwright test \
  --grep-invert "@hardware" tests/e2e/org-crud.spec.ts
```

Per the "black-box verification is batched" workflow, this is folded into the post-series black-box pass — not run per-ticket.

Local development checks:

```bash
just frontend test:e2e tests/e2e/org-crud.spec.ts
just frontend test
just frontend typecheck
just frontend lint
```

## Out of scope

- `inventory-save.spec.ts:346 › switching orgs clears asset mappings` — uses `switchOrgViaAPI`, not the broken UI helper. Misattributed in the original issue. Gets a separate ticket if still red after this lands.
- Showing the current org name on the switcher button (UX improvement). Possible but unrelated to the broken-tests problem.
- Migrating UI callers of `switchToOrg` to `switchOrgViaAPI`. Both helpers exist; choice stays per-test.
