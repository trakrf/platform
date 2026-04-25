# TRA-497: E2E org-crud fixture stable signal — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore green for the 6 failing `org-crud.spec.ts` tests by giving the OrgSwitcher button a stable model-truth signal (`data-current-org-id` and `data-current-org-name`), rewriting the broken `switchToOrg` helper to wait on that signal, and replacing the bogus `testEmail` menuitem assertion with the real signup org name.

**Architecture:** Three files change. (1) `frontend/src/components/OrgSwitcher.tsx` — add two `data-*` attributes to the existing `Menu.Button`, sourced from `useOrgStore().currentOrg`. (2) `frontend/tests/e2e/fixtures/org.fixture.ts` — rewrite `switchToOrg` body to wait on `data-current-org-name` instead of the impossible `:has-text(orgName)` selector. (3) `frontend/tests/e2e/org-crud.spec.ts` — wire the dead `testOrgName` variable into `signupTestUser` and swap the `testEmail` menuitem assertion for `testOrgName`. No backend changes, no new modules, no UX changes (the avatar still shows only the email initial).

**Tech Stack:** TypeScript, React (Headless UI Menu), Zustand store, Playwright, pnpm, Vite. Run from repo root via `just frontend <cmd>` delegates per project CLAUDE.md.

---

## File Structure

**Modify:**
- `frontend/src/components/OrgSwitcher.tsx:64-68` — add two attributes to `Menu.Button`
- `frontend/tests/e2e/fixtures/org.fixture.ts:104-115` — rewrite `switchToOrg` body
- `frontend/tests/e2e/org-crud.spec.ts:47,115` — pass `testOrgName` into signup, swap menuitem assertion

**No files created or deleted.**

---

## Task 1: Rename branch to project convention

**Files:** none — git operation only.

The worktree was created on `worktree-tra-497`. The PR convention (matching Linear's suggested branch name) is `miks2u/tra-497-e2e-org-crud-helpers-fail-switcher-button-text-and-personal`. Rename before pushing so the branch on the remote matches Linear.

- [ ] **Step 1: Confirm current branch and rename**

```bash
git branch --show-current
# Expected: worktree-tra-497

git branch -m miks2u/tra-497-e2e-org-crud-helpers-fail-switcher-button-text-and-personal
git branch --show-current
# Expected: miks2u/tra-497-e2e-org-crud-helpers-fail-switcher-button-text-and-personal
```

No commit — branch rename is local metadata.

---

## Task 2: Add `data-current-org-*` attributes to OrgSwitcher

**Files:**
- Modify: `frontend/src/components/OrgSwitcher.tsx:64-68`

Both values come from `currentOrg` which is already destructured from `useOrgStore()` at line 25. Falling back to empty string (rather than omitting the attribute) keeps DOM shape consistent so future tests can wait on a non-empty value as a "loaded" signal.

- [ ] **Step 1: Add two attributes to Menu.Button**

Find (lines 64-68):

```tsx
      <Menu.Button
        disabled={isLoading}
        data-testid="org-switcher"
        className="flex items-center gap-1.5 px-2 py-1.5 rounded-md hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors disabled:opacity-50"
      >
```

Replace with:

```tsx
      <Menu.Button
        disabled={isLoading}
        data-testid="org-switcher"
        data-current-org-id={currentOrg?.id ?? ''}
        data-current-org-name={currentOrg?.name ?? ''}
        className="flex items-center gap-1.5 px-2 py-1.5 rounded-md hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors disabled:opacity-50"
      >
```

- [ ] **Step 2: Run typecheck and lint**

Run from repo root:

```bash
just frontend typecheck
just frontend lint
```

Expected: both pass with no errors. (TypeScript may infer `data-current-org-id={...}` as the required `string | number`-compatible type via React's intrinsic attributes; the `?? ''` fallback ensures we never pass `undefined`.)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/OrgSwitcher.tsx
git commit -m "$(cat <<'EOF'
feat(tra-497): expose current org id and name as data-* attrs on switcher

Adds data-current-org-id and data-current-org-name to OrgSwitcher's
Menu.Button so e2e tests can wait on model truth instead of rendered
button text. The button's visible UI is unchanged — it still shows
only the email initial avatar; these attributes are pure test surface.

Empty string fallback (instead of omitting) keeps DOM shape consistent
when there is no current org, so tests can reliably wait on a
non-empty value as a "loaded" signal.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Rewrite `switchToOrg` helper to wait on model truth

**Files:**
- Modify: `frontend/tests/e2e/fixtures/org.fixture.ts:101-115`

The current helper has two flaws: (a) the click locator `button:has-text(orgName)` is too loose, and (b) the wait selector `[data-testid="org-switcher"]:has-text(orgName)` cannot match because the button only renders the email initial. The rewrite tightens the click to `getByRole('menuitem', {name})` and waits on the new `data-current-org-name` attribute.

- [ ] **Step 1: Replace the body of `switchToOrg`**

Find (lines 101-115):

```ts
/**
 * Switch to a specific org via the org switcher dropdown
 * Waits for the switcher to update to show the new org name
 */
export async function switchToOrg(page: Page, orgName: string): Promise<void> {
  await openOrgSwitcher(page);
  // Click the org in the dropdown menu
  await page.locator(`button:has-text("${orgName}")`).click();
  // Wait for dropdown to close (menu items disappear)
  await page.waitForSelector('[role="menu"]', { state: 'hidden', timeout: 5000 });

  // Wait for UI to reflect the new org - the switcher button should show the org name
  await page.waitForSelector(`[data-testid="org-switcher"]:has-text("${orgName}")`, {
    timeout: 10000,
  });
}
```

Replace with:

```ts
/**
 * Switch to a specific org via the org switcher dropdown.
 * Waits for the switcher button's data-current-org-name attribute to
 * reflect the new org — this is model truth from the zustand store, not
 * rendered button text (the button only shows the user's email initial).
 */
export async function switchToOrg(page: Page, orgName: string): Promise<void> {
  await openOrgSwitcher(page);
  // Click the org in the dropdown menu
  await page.getByRole('menuitem', { name: orgName }).click();

  // Wait for the switcher to reflect the new current org.
  // Headless UI unmounts the menu on click; the attribute settle implicitly
  // proves both that the dropdown closed and the switch resolved.
  await page.waitForSelector(
    `[data-testid="org-switcher"][data-current-org-name="${orgName}"]`,
    { timeout: 10000 }
  );
}
```

- [ ] **Step 2: Run typecheck and lint**

```bash
just frontend typecheck
just frontend lint
```

Expected: both pass.

- [ ] **Step 3: Commit**

```bash
git add frontend/tests/e2e/fixtures/org.fixture.ts
git commit -m "$(cat <<'EOF'
fix(tra-497): switchToOrg waits on data-current-org-name attribute

The previous implementation waited on
[data-testid="org-switcher"]:has-text(orgName), but the switcher button
renders only the user's email initial (no org name in its text), so the
selector could never match — every switch hit the 10s timeout.

Now waits on the data-current-org-name attribute exposed in TRA-497
(see preceding commit), which reflects the zustand store state directly.
Also tightens the click locator from button:has-text() to
getByRole('menuitem', {name}) to match how the listing test asserts.

Drops the redundant [role=menu] hidden wait — Headless UI unmounts the
menu after click, and the attribute settle is a stronger composite
signal (dropdown closed AND switch resolved AND store updated).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Wire `testOrgName` into signup and swap menuitem assertion

**Files:**
- Modify: `frontend/tests/e2e/org-crud.spec.ts:47,115`

Two surgical edits. (4a) The variable `testOrgName` is constructed at line 42 but never passed to `signupTestUser` — fix that. (4b) The listing test asserts a menuitem named after `testEmail`, which the product never creates; assert on `testOrgName` instead.

- [ ] **Step 1: Pass `testOrgName` to `signupTestUser`**

Find (line 47):

```ts
    await signupTestUser(page, testEmail, testPassword);
```

Replace with:

```ts
    await signupTestUser(page, testEmail, testPassword, testOrgName);
```

- [ ] **Step 2: Replace the email-named menuitem assertion**

Find (lines 114-115):

```ts
      // Should see the personal org from signup (named after email)
      await expect(page.getByRole('menuitem', { name: testEmail })).toBeVisible();
```

Replace with:

```ts
      // Should see the org from signup
      await expect(page.getByRole('menuitem', { name: testOrgName })).toBeVisible();
```

- [ ] **Step 3: Run typecheck and lint**

```bash
just frontend typecheck
just frontend lint
```

Expected: both pass.

- [ ] **Step 4: Commit**

```bash
git add frontend/tests/e2e/org-crud.spec.ts
git commit -m "$(cat <<'EOF'
fix(tra-497): wire testOrgName into signup, drop bogus testEmail assertion

Two surgical fixes to org-crud.spec.ts:

1. testOrgName was constructed in beforeAll but never passed to
   signupTestUser, so the helper fell back to its own unique default and
   the test had no way to know what name the signup org actually got.
   Wire the variable through so the assertion in the listing test has
   something deterministic to look for.

2. The "should display orgs in switcher dropdown" test asserted that
   a menuitem named after testEmail was visible. The product never
   creates a personal org named after the user's email; the dropdown
   only lists org.name values. Swap to assert the (now properly wired)
   testOrgName instead, matching the test's stated purpose.

The Edit/Delete tests in this spec already call switchToOrg(page, name)
and become green via the helper fix in the preceding commit.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Run the wider validation suite

**Files:** none — validation only.

The data-attribute change is small but touches a component used in `Header` and possibly other places. Run the broader frontend checks to make sure nothing regressed.

- [ ] **Step 1: Frontend unit tests**

```bash
just frontend test
```

Expected: all pass. There is no `OrgSwitcher.test.tsx`; the only related unit test is `Header.test.tsx`, which exercises the header's wiring of `OrgSwitcher` — adding two data attributes shouldn't change anything it asserts on.

- [ ] **Step 2: Full typecheck and lint sweep**

```bash
just frontend typecheck
just frontend lint
```

Expected: both pass.

- [ ] **Step 3: (Optional) Local e2e smoke against the targeted spec**

If a local backend is running and you want to confirm the fix locally before relying on the post-series black-box pass:

```bash
just frontend test:e2e tests/e2e/org-crud.spec.ts
```

Expected: 6 previously-failing tests now pass. (Per project memory, full black-box verification against `gke.trakrf.app` is batched at the end of the ticket series — not run per-ticket — so skipping this step is fine.)

No commit — validation only.

---

## Task 6: Push and open PR

**Files:** none — git/gh operations only.

- [ ] **Step 1: Push branch with upstream tracking**

```bash
git push -u origin miks2u/tra-497-e2e-org-crud-helpers-fail-switcher-button-text-and-personal
```

Expected: push succeeds, branch is now tracking the remote.

- [ ] **Step 2: Open PR via gh**

```bash
gh pr create \
  --title "fix(tra-497): e2e org-crud — stable switcher signal + correct listing assertion" \
  --body "$(cat <<'EOF'
## Summary

Restores green for 6 failing tests in `frontend/tests/e2e/org-crud.spec.ts` by giving the `OrgSwitcher` button a stable model-truth signal (`data-current-org-id` and `data-current-org-name`), rewriting the broken `switchToOrg` helper to wait on that signal, and replacing the bogus `testEmail` menuitem assertion with the real signup org name.

- **Product:** Two `data-*` attributes added to `OrgSwitcher`'s `Menu.Button`. Visible UI unchanged.
- **Test fixture:** `switchToOrg` now waits on `data-current-org-name`; click locator tightened to `getByRole('menuitem', {name})`.
- **Test spec:** `testOrgName` is now passed into `signupTestUser` (was dead code); listing assertion swapped from `testEmail` to `testOrgName`.

`inventory-save.spec.ts:346` was excluded — it uses `switchOrgViaAPI` (not the broken UI helper) and was misattributed in the original issue.

Spec: `docs/superpowers/specs/2026-04-25-tra-497-e2e-org-fixture-design.md`
Plan: `docs/superpowers/plans/2026-04-25-tra-497-e2e-org-fixture.md`

## Test plan

- [ ] `just frontend typecheck` clean
- [ ] `just frontend lint` clean
- [ ] `just frontend test` green (Header.test.tsx unaffected)
- [ ] Post-series black-box pass against `gke.trakrf.app` confirms all 6 org-crud tests green

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR URL printed. Capture and report it.

No commit — operation is push + PR creation.

---

## Self-review notes

- Spec coverage: every section in the design (product attribute, fixture rewrite, spec edits, verification, scope exclusion of `inventory-save`) is mapped to a task.
- Placeholders: none.
- Type consistency: `data-current-org-id={currentOrg?.id ?? ''}` and `data-current-org-name={currentOrg?.name ?? ''}` — values originate from the same `currentOrg` destructure already in scope at `OrgSwitcher.tsx:25`. Helper still takes `(page: Page, orgName: string)` — no signature changes, so all 5 existing callers in `org-crud.spec.ts` continue to work without edits.
- Commit count: 3 functional commits (one per file change), matching the project's "prefer incremental commits" preference.
