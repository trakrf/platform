# TRA-341 Reconcile Auth Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When an unauthenticated user taps the Reconcile Upload CSV button on the inventory screen, show an upsell toast and redirect to login instead of opening the file picker. Also remove the redundant Download Sample CSV menu item (Share/Download covers it).

**Architecture:** Add a `handleReconcileUpload` callback in `InventoryScreen.tsx` that mirrors the existing `handleSave` auth-gate pattern (lines 229-234), but adds a `react-hot-toast` notification before redirect. Replace the inline `onUploadCSV` prop wiring to use the new callback. Delete the Download Sample menu items (mobile + desktop) from `InventoryHeader.tsx`, drop the `onDownloadSample` prop, remove the pass-through in `InventoryScreen.tsx`, and delete the unused `downloadSampleReconFile` from `useReconciliation.ts`.

**Tech Stack:** React 18 + TypeScript + Vite, Zustand (`useAuthStore`), `react-hot-toast` (default import `toast`), Vitest + `@testing-library/react` (jsdom).

**Spec:** [docs/superpowers/specs/2026-04-18-tra341-reconcile-auth-gate-design.md](../specs/2026-04-18-tra341-reconcile-auth-gate-design.md)

**Branch:** `feature/tra-341-reconcile-auth-gate` (already created). **Worktree:** `.worktrees/tra-341`.

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `frontend/src/components/InventoryScreen.tsx` | MODIFY | Add `handleReconcileUpload` callback that gates the click behind `isAuthenticated`. Rewire `onUploadCSV`. Drop the `onDownloadSample` prop pass-through. Remove `downloadSampleReconFile` from the `useReconciliation` destructure. Add `import toast from 'react-hot-toast'`. |
| `frontend/src/components/inventory/InventoryHeader.tsx` | MODIFY | Drop `onDownloadSample` from `InventoryHeaderProps`. Remove the Download Sample `<button>` (and its containing wrapper fix-ups) in both mobile and desktop layouts. Drop the unused `Download` import. |
| `frontend/src/hooks/useReconciliation.ts` | MODIFY | Delete `downloadSampleReconFile` callback; drop it from the return object. Drop the now-unused `SAMPLE_INVENTORY_DATA` import. |
| `frontend/src/components/__tests__/InventoryScreen.test.tsx` | MODIFY | Add a new `describe('InventoryScreen Reconcile auth gate')` block with three tests (unauth gate, auth passthrough, menu-item removed). Mock `react-hot-toast`. |

---

## Conventions used in this plan

- **Working directory:** commands run from repo root (`/home/mike/platform/.worktrees/tra-341`). Use `just frontend <recipe>` for workspace-scoped commands.
- **Tests run with:** `just frontend test` (Vitest, jsdom).
- **Commits:** Conventional commits — `feat(tra-341): ...`, `refactor(tra-341): ...`, `test(tra-341): ...`.
- **TDD:** Each behavioral change has its failing test written and verified failing BEFORE the implementation change.

---

## Task 1: Install frontend dependencies and verify clean baseline

**Files:** none

- [ ] **Step 1: Install deps in the worktree**

Run from `.worktrees/tra-341`:

```bash
pnpm -C frontend install
```

Expected: pnpm installs packages, no errors. (First run in a fresh worktree — `node_modules/` doesn't exist yet.) There is no `just frontend install` recipe today; the `-C` flag sets pnpm's working directory without shell `cd`.

- [ ] **Step 2: Run existing tests to establish a clean baseline**

```bash
just frontend test
```

Expected: all existing tests PASS with 0 failures.

If failures exist: stop — report the failing tests and ask before proceeding. Pre-existing failures must be triaged separately; we should not mix them with TRA-341 work.

- [ ] **Step 3: Run typecheck + lint baseline**

```bash
just frontend typecheck
just frontend lint
```

Expected: both clean.

---

## Task 2: Add failing test — unauthenticated click shows toast and redirects

**Files:**
- Modify: `frontend/src/components/__tests__/InventoryScreen.test.tsx`

- [ ] **Step 1: Mock `react-hot-toast` at the top of the test file and add the new describe block**

Add the mock just below the existing imports (before `generateTestTags`):

```ts
import toast from 'react-hot-toast';
import { useAuthStore } from '@/stores';

vi.mock('react-hot-toast', () => ({
  default: vi.fn(),
}));
```

Append this `describe` block at the bottom of the file (after the pagination `describe` closes, after the trailing `// TODO: Add export test...` comment):

```ts
describe('InventoryScreen Reconcile auth gate', () => {
  afterEach(() => {
    cleanup();
    vi.mocked(toast).mockClear();
    sessionStorage.clear();
    window.location.hash = '';
  });

  beforeEach(() => {
    useTagStore.getState().clearTags();
    useAuthStore.setState({ isAuthenticated: false, token: null, user: null });
  });

  it('shows upsell toast and redirects to login when an unauthenticated user clicks Reconcile', async () => {
    const clickSpy = vi.spyOn(HTMLInputElement.prototype, 'click');

    render(<InventoryScreen />);

    // Desktop layout renders a labeled "Reconcile" button
    const reconcileButton = screen.getAllByRole('button', { name: /reconcile/i })[0];
    fireEvent.click(reconcileButton);

    await waitFor(() => {
      expect(toast).toHaveBeenCalledWith(
        'Reconciliation is a paid feature. Log in to start your free trial.'
      );
    });
    expect(sessionStorage.getItem('redirectAfterLogin')).toBe('inventory');
    expect(window.location.hash).toBe('#login');
    expect(clickSpy).not.toHaveBeenCalled();

    clickSpy.mockRestore();
  });
});
```

- [ ] **Step 2: Run the new test to verify it fails**

```bash
pnpm -C frontend test -- InventoryScreen.test
```

Expected: the new test FAILS — either because `toast` was never called, or the file input's click fires, or both. (Gate logic doesn't exist yet.)

Do NOT commit yet — commit after the implementation makes it pass (Task 4).

---

## Task 3: Add second failing test — authenticated click proceeds as before

**Files:**
- Modify: `frontend/src/components/__tests__/InventoryScreen.test.tsx`

- [ ] **Step 1: Append the authenticated-passthrough test inside the same `describe('InventoryScreen Reconcile auth gate')` block**

Add this `it` block immediately after the previous test:

```ts
  it('opens file picker for authenticated users without toasting', async () => {
    useAuthStore.setState({ isAuthenticated: true, token: 'test-token', user: { id: 1, email: 't@e.st' } as never });
    const clickSpy = vi.spyOn(HTMLInputElement.prototype, 'click');

    render(<InventoryScreen />);

    const reconcileButton = screen.getAllByRole('button', { name: /reconcile/i })[0];
    fireEvent.click(reconcileButton);

    expect(clickSpy).toHaveBeenCalledTimes(1);
    expect(toast).not.toHaveBeenCalled();
    expect(window.location.hash).toBe('');
    expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();

    clickSpy.mockRestore();
  });
```

- [ ] **Step 2: Run tests — expect the new test to also FAIL or the unauth test to still fail**

```bash
pnpm -C frontend test -- InventoryScreen.test
```

Expected: unauth test FAILS (no gate yet); this authed test currently PASSES coincidentally because the current code always calls `fileInputRef.current?.click()`. Either way we haven't committed, and both tests will pass after Task 4.

---

## Task 4: Implement the auth gate in `InventoryScreen.tsx`

**Files:**
- Modify: `frontend/src/components/InventoryScreen.tsx`

- [ ] **Step 1: Add the `react-hot-toast` import**

At the top of the file, add (alphabetical/grouping is loose in this file — place near the other `@/` imports is fine):

```ts
import toast from 'react-hot-toast';
```

- [ ] **Step 2: Add the `handleReconcileUpload` callback**

Add this `useCallback` immediately after the `handleSave` definition (which ends at line 262, closing `}, [isAuthenticated, resolvedLocation, ...])`):

```ts
  const handleReconcileUpload = useCallback(() => {
    if (!isAuthenticated) {
      toast('Reconciliation is a paid feature. Log in to start your free trial.');
      sessionStorage.setItem('redirectAfterLogin', 'inventory');
      window.location.hash = '#login';
      return;
    }
    fileInputRef.current?.click();
  }, [isAuthenticated]);
```

- [ ] **Step 3: Rewire the `onUploadCSV` prop on `<InventoryHeader>`**

Find line 294:

```tsx
          onUploadCSV={() => fileInputRef.current?.click()}
```

Replace with:

```tsx
          onUploadCSV={handleReconcileUpload}
```

- [ ] **Step 4: Run the two new tests — expect PASS**

```bash
pnpm -C frontend test -- InventoryScreen.test
```

Expected: all tests in `InventoryScreen.test.tsx` PASS, including the two new auth-gate tests.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/InventoryScreen.tsx frontend/src/components/__tests__/InventoryScreen.test.tsx
git commit -m "feat(tra-341): gate reconcile upload behind auth with upsell toast"
```

---

## Task 5: Add failing test — Download Sample menu item is absent

**Files:**
- Modify: `frontend/src/components/__tests__/InventoryScreen.test.tsx`

- [ ] **Step 1: Append the menu-absence test inside the same `describe('InventoryScreen Reconcile auth gate')` block**

```ts
  it('does not render a Download Sample menu item', () => {
    render(<InventoryScreen />);

    expect(screen.queryByRole('button', { name: /sample/i })).not.toBeInTheDocument();
    expect(screen.queryByTitle(/download sample/i)).not.toBeInTheDocument();
  });
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
pnpm -C frontend test -- InventoryScreen.test
```

Expected: this test FAILS because the Download Sample button still exists in both desktop (labeled "Sample") and mobile (`title="Download sample CSV"`) layouts.

---

## Task 6: Remove the Download Sample menu item from `InventoryHeader.tsx`

**Files:**
- Modify: `frontend/src/components/inventory/InventoryHeader.tsx`

- [ ] **Step 1: Drop `onDownloadSample` from the props interface**

Remove this line from `InventoryHeaderProps` (currently line 12):

```ts
  onDownloadSample: () => void;
```

- [ ] **Step 2: Drop `onDownloadSample` from the destructured props**

Remove this line from the `InventoryHeader` function signature (currently line 36):

```ts
  onDownloadSample,
```

- [ ] **Step 3: Remove the mobile-layout Download Sample button and collapse the flex wrapper**

In the mobile branch, the current markup is:

```tsx
            <div className="flex">
              <button
                onClick={onDownloadSample}
                className="p-1.5 sm:p-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-l-lg transition-colors border-r border-gray-200 dark:border-gray-600"
                title="Download sample CSV"
              >
                <Download className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
              </button>
              <button
                onClick={onUploadCSV}
                disabled={isProcessingCSV}
                className="p-1.5 sm:p-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-r-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                title="Upload reconciliation CSV"
              >
                <Upload className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
              </button>
            </div>
```

Replace with a single standalone button (no wrapper div needed, now that it's one button):

```tsx
            <button
              onClick={onUploadCSV}
              disabled={isProcessingCSV}
              className="p-1.5 sm:p-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              title="Upload reconciliation CSV"
            >
              <Upload className="w-3.5 h-3.5 sm:w-4 sm:h-4" />
            </button>
```

(Note the className change: `rounded-r-lg` → `rounded-lg` — no longer part of a paired group, so it gets full rounding.)

- [ ] **Step 4: Remove the desktop-layout Download Sample button and collapse the flex wrapper**

In the desktop branch, the current markup is:

```tsx
          <div className="flex">
            <button
              onClick={onDownloadSample}
              className="px-3 py-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-l-lg font-medium transition-colors flex items-center text-sm border-r border-gray-200 dark:border-gray-600"
            >
              <Download className="w-4 h-4 mr-1.5" />
              Sample
            </button>
            <button
              onClick={onUploadCSV}
              disabled={isProcessingCSV}
              className="px-3 py-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-r-lg font-medium transition-colors flex items-center text-sm disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <Upload className="w-4 h-4 mr-1.5" />
              Reconcile
            </button>
          </div>
```

Replace with a single standalone button:

```tsx
          <button
            onClick={onUploadCSV}
            disabled={isProcessingCSV}
            className="px-3 py-2 bg-gray-100 dark:bg-gray-700 hover:bg-gray-200 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 rounded-lg font-medium transition-colors flex items-center text-sm disabled:opacity-50 disabled:cursor-not-allowed"
          >
            <Upload className="w-4 h-4 mr-1.5" />
            Reconcile
          </button>
```

(Same className change: `rounded-r-lg` → `rounded-lg`.)

- [ ] **Step 5: Remove the unused `Download` import**

The top-of-file import is currently:

```ts
import { Download, Package2, Trash2, Upload, Volume2, VolumeX, Play, Pause, Save } from 'lucide-react';
```

Remove `Download`:

```ts
import { Package2, Trash2, Upload, Volume2, VolumeX, Play, Pause, Save } from 'lucide-react';
```

---

## Task 7: Drop the `onDownloadSample` pass-through in `InventoryScreen.tsx`

**Files:**
- Modify: `frontend/src/components/InventoryScreen.tsx`

- [ ] **Step 1: Remove the prop pass-through**

Line 293 currently reads:

```tsx
          onDownloadSample={downloadSampleReconFile}
```

Delete that line entirely.

- [ ] **Step 2: Drop `downloadSampleReconFile` from the `useReconciliation` destructure**

Line 60 currently reads:

```ts
  const { error, setError, isProcessingCSV, fileInputRef, handleReconciliationUpload, downloadSampleReconFile } = useReconciliation();
```

Change to:

```ts
  const { error, setError, isProcessingCSV, fileInputRef, handleReconciliationUpload } = useReconciliation();
```

---

## Task 8: Delete `downloadSampleReconFile` from `useReconciliation.ts`

**Files:**
- Modify: `frontend/src/hooks/useReconciliation.ts`

- [ ] **Step 1: Remove the callback definition**

Delete lines 49-65 — the entire `downloadSampleReconFile` `useCallback` block:

```ts
  const downloadSampleReconFile = useCallback(() => {
    try {
      const sampleContent = `Tag ID,Description,Location\n${SAMPLE_INVENTORY_DATA.map(item => `${item.epc},${item.description},${item.location}`).join('\n')}`;
      const blob = new Blob([sampleContent], { type: 'text/csv' });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = 'reconciliation_sample.csv';
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error('Error downloading sample file:', error);
      setError('Failed to download sample file');
    }
  }, []);
```

- [ ] **Step 2: Drop the function from the return object**

The return currently is:

```ts
  return {
    error,
    setError,
    isProcessingCSV,
    fileInputRef,
    handleReconciliationUpload,
    downloadSampleReconFile,
  };
```

Change to:

```ts
  return {
    error,
    setError,
    isProcessingCSV,
    fileInputRef,
    handleReconciliationUpload,
  };
```

- [ ] **Step 3: Drop the now-unused `SAMPLE_INVENTORY_DATA` import**

Line 4 currently is:

```ts
import { SAMPLE_INVENTORY_DATA } from '@test-utils/constants';
```

Delete that import.

- [ ] **Step 4: Run all frontend tests to confirm the menu-absence test passes and nothing regressed**

```bash
just frontend test
```

Expected: all tests PASS, including the Download-Sample-absence test added in Task 5.

- [ ] **Step 5: Run typecheck and lint**

```bash
just frontend typecheck
just frontend lint
```

Expected: clean. (If an unused-import or unused-variable warning appears, address it — don't ignore.)

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/inventory/InventoryHeader.tsx \
        frontend/src/components/InventoryScreen.tsx \
        frontend/src/hooks/useReconciliation.ts \
        frontend/src/components/__tests__/InventoryScreen.test.tsx
git commit -m "refactor(tra-341): remove redundant Download Sample menu item"
```

---

## Task 9: Full validation sweep

**Files:** none

- [ ] **Step 1: Run the combined validator**

```bash
just validate
```

Expected: frontend `lint` + `typecheck` + `test` pass; backend checks pass unchanged. If backend fails for unrelated reasons, report but do not attempt a fix in this branch.

- [ ] **Step 2: Sanity-check the git log**

```bash
git log --oneline main..HEAD
```

Expected: three commits — the spec doc commit from brainstorming, plus Tasks 4 and 8's commits.

---

## Task 10: Open the PR

**Files:** none (GitHub metadata only)

- [ ] **Step 1: Push the branch**

```bash
git push -u origin feature/tra-341-reconcile-auth-gate
```

- [ ] **Step 2: Open the PR with `gh`**

```bash
gh pr create --title "feat(tra-341): gate reconcile upload behind auth + remove sample CSV" --body "$(cat <<'EOF'
## Summary
- Free/unauthenticated users tapping Reconcile Upload CSV now see an upsell toast ("Reconciliation is a paid feature. Log in to start your free trial.") and are redirected to login; after login they return to the inventory screen.
- Button stays visible to free users for discovery.
- Removes the redundant Download Sample CSV menu item — Share/Download covers the same need.

Closes TRA-341. Spec: docs/superpowers/specs/2026-04-18-tra341-reconcile-auth-gate-design.md.

## Test plan
- [ ] `just validate` passes locally
- [ ] Preview deploy: logged out on `https://app.preview.trakrf.id`, tap Reconcile → see upsell toast → land on login
- [ ] Log in → return to inventory
- [ ] Logged-in user: tap Reconcile → file picker opens as before
- [ ] Menu no longer shows Download Sample in mobile or desktop layouts

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR opens against `main`. Preview deploy kicks off via `.github/workflows/sync-preview.yml`.

- [ ] **Step 3: Manual verification in preview**

Once preview is up at `https://app.preview.trakrf.id`:

1. Sign out (or open an incognito window).
2. Navigate to the Inventory screen.
3. Click **Reconcile** (desktop) or the upload icon (mobile).
4. Confirm: toast copy matches spec; browser redirects to `#login`.
5. Log in.
6. Confirm: returned to inventory screen (not home).
7. Click **Reconcile** again: file picker opens.
8. Scan the header: no Download Sample button visible.

If any step fails, document the failure in the PR and fix in a follow-up commit before merge.
